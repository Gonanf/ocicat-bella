package handlers

import (
	"bytes"
	"context"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// MaxFileSize: límite §0 (10 MB por archivo).
const MaxFileSize = 10 << 20

func validVisibility(v model.Visibility) bool {
	return v == model.VisPublic || v == model.VisSchool || v == model.VisClassroom
}

// materialKind deriva el type del listado §6 desde la extensión.
func materialKind(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "pdf"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp":
		return "image"
	case ".mp4", ".webm", ".ogg", ".mov", ".mkv":
		return "video"
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp":
		return "office"
	default:
		return "other"
	}
}

func previewAvailable(kind string) bool {
	return kind == "pdf" || kind == "image" || kind == "video"
}

// materialAccess reúne lo que hace falta para decidir visibilidad sin N consultas.
type materialAccess struct {
	user     *model.User
	memberOf map[string]bool // aulas donde el que pregunta es miembro
}

func (h *Handler) accessFor(r *http.Request) (*materialAccess, error) {
	a := &materialAccess{user: middleware.UserFromContext(r.Context())}
	if a.user != nil {
		mems, err := h.store.ListMembershipsByUser(r.Context(), a.user.ID)
		if err != nil {
			return nil, err
		}
		a.memberOf = make(map[string]bool, len(mems))
		for _, mem := range mems {
			a.memberOf[mem.ClassroomID] = true
		}
	}
	return a, nil
}

// canSeeMaterial aplica las reglas de visibilidad §6:
// public → todos; school → cualquier autenticado (invitado incluido);
// classroom → director, autor, docente del aula o miembro del aula.
func (h *Handler) canSeeMaterial(ctx context.Context, a *materialAccess, m *model.Material) bool {
	switch m.Visibility {
	case model.VisPublic:
		return true
	case model.VisSchool:
		return a.user != nil
	case model.VisClassroom:
		if a.user == nil {
			return false
		}
		switch {
		case a.user.Role == model.RoleDirector, m.UploadedBy == a.user.ID:
			return true
		case a.memberOf[m.ClassroomID]:
			return true
		}
		c, err := h.store.GetClassroom(ctx, m.ClassroomID)
		return err == nil && c.TeacherID == a.user.ID
	}
	return false
}

func materialView(m *model.Material) map[string]any {
	kind := materialKind(m.Filename)
	return map[string]any{
		"id":                m.ID,
		"title":             m.Title,
		"type":              kind,
		"visibility":        m.Visibility,
		"subject":           m.Subject,
		"preview_available": previewAvailable(kind),
		"uploaded_by":       m.UploadedBy,
		"classroom_id":      m.ClassroomID,
		"filename":          m.Filename,
		"content_type":      m.ContentType,
		"size":              m.Size,
		"created_at":        m.CreatedAt,
	}
}

// ListMaterials maneja GET /materials?scope=public|mine|classroom:{id}&subject=
// (§6): pública; filtra según quien pregunta.
func (h *Handler) ListMaterials(w http.ResponseWriter, r *http.Request) {
	a, err := h.accessFor(r)
	if err != nil {
		writeInternal(w)
		return
	}

	scope := r.URL.Query().Get("scope")
	var scopeClassroom string
	if strings.HasPrefix(scope, "classroom:") {
		scopeClassroom = strings.TrimPrefix(scope, "classroom:")
	} else if scope != "" && scope != "public" && scope != "mine" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))

	all, err := h.store.ListMaterials(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}

	items := []map[string]any{}
	for i := range all {
		m := &all[i]
		if !h.canSeeMaterial(r.Context(), a, m) {
			continue
		}
		switch scope {
		case "mine":
			if a.user == nil || m.UploadedBy != a.user.ID {
				continue
			}
		default:
			if scopeClassroom != "" && m.ClassroomID != scopeClassroom {
				continue
			}
		}
		if subject != "" && !strings.EqualFold(m.Subject, subject) {
			continue
		}
		items = append(items, materialView(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": items})
}

// UploadMaterial maneja POST /classrooms/{id}/materials (§6): multipart
// file,title,visibility,subject? — docente dueño o director.
func (h *Handler) UploadMaterial(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.GetClassroom(r.Context(), r.PathValue("id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	user := middleware.UserFromContext(r.Context())
	if !canManageClassroom(user, c) {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			errors.WriteCode(w, errors.CodeFileTooLarge)
			return
		}
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	vis := model.Visibility(r.FormValue("visibility"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	if title == "" || !validVisibility(vis) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil || header.Size > MaxFileSize {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	defer file.Close()

	data := make([]byte, 0, header.Size)
	buf := make([]byte, 32<<10)
	total := 0
	for {
		n, rerr := file.Read(buf)
		data = append(data, buf[:n]...)
		total += n
		if total > MaxFileSize {
			errors.WriteCode(w, errors.CodeFileTooLarge)
			return
		}
		if rerr != nil {
			break
		}
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		if ct := mime.TypeByExtension(filepath.Ext(header.Filename)); ct != "" {
			contentType = ct
		}
	}

	m := &model.Material{
		ID:          newID(),
		ClassroomID: c.ID,
		UploadedBy:  user.ID,
		Title:       title,
		Filename:    filepath.Base(header.Filename),
		ContentType: contentType,
		Size:        int64(len(data)),
		Visibility:  vis,
		Subject:     subject,
		Data:        data,
		CreatedAt:   time.Now(),
	}
	if err := h.store.CreateMaterial(r.Context(), m); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, materialView(m))
}

// loadMaterialVisible resuelve el material y aplica 404 indistinguible (§6).
func (h *Handler) loadMaterialVisible(w http.ResponseWriter, r *http.Request) *model.Material {
	m, err := h.store.GetMaterial(r.Context(), r.PathValue("id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil
	}
	a, aerr := h.accessFor(r)
	if aerr != nil {
		writeInternal(w)
		return nil
	}
	if !h.canSeeMaterial(r.Context(), a, m) {
		errors.WriteCode(w, errors.CodeNotFound) // indistinguible de inexistente
		return nil
	}
	return m
}

// GetMaterialMeta maneja GET /materials/{id}.
func (h *Handler) GetMaterialMeta(w http.ResponseWriter, r *http.Request) {
	m := h.loadMaterialVisible(w, r)
	if m == nil {
		return
	}
	writeJSON(w, http.StatusOK, materialView(m))
}

// ServeMaterialFile maneja GET /materials/{id}/file con Range y ?download=1 (§6).
// http.ServeContent resuelve Range/If-Range/206 por nosotros.
func (h *Handler) ServeMaterialFile(w http.ResponseWriter, r *http.Request) {
	m := h.loadMaterialVisible(w, r)
	if m == nil {
		return
	}
	name := m.Filename
	if name == "" {
		name = m.Title
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	http.ServeContent(w, r, name, m.CreatedAt, bytes.NewReader(m.Data))
}

// PatchMaterial maneja PATCH /materials/{id}: autor docente o director (§6).
func (h *Handler) PatchMaterial(w http.ResponseWriter, r *http.Request) {
	m := h.loadMaterialVisible(w, r)
	if m == nil {
		return
	}
	user := middleware.UserFromContext(r.Context())
	if user.Role != model.RoleDirector && m.UploadedBy != user.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}
	var req struct {
		Title      *string `json:"title"`
		Visibility *string `json:"visibility"`
		Subject    *string `json:"subject"`
	}
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		m.Title = strings.TrimSpace(*req.Title)
	}
	if req.Visibility != nil {
		v := model.Visibility(*req.Visibility)
		if !validVisibility(v) {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		m.Visibility = v
	}
	if req.Subject != nil {
		m.Subject = strings.TrimSpace(*req.Subject)
	}
	if err := h.store.UpdateMaterial(r.Context(), m); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, materialView(m))
}

// DeleteMaterial maneja DELETE /materials/{id}: autor docente o director.
func (h *Handler) DeleteMaterial(w http.ResponseWriter, r *http.Request) {
	m := h.loadMaterialVisible(w, r)
	if m == nil {
		return
	}
	user := middleware.UserFromContext(r.Context())
	if user.Role != model.RoleDirector && m.UploadedBy != user.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}
	if err := h.store.DeleteMaterial(r.Context(), m.ID); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
