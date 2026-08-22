/* Ocicat Bella — interacciones mínimas, sin frameworks */
(function () {
  'use strict';
  var $ = function (sel, el) { return (el || document).querySelector(sel); };
  var $$ = function (sel, el) { return Array.prototype.slice.call((el || document).querySelectorAll(sel)); };

  /* Materiales: filtro combinado de materia + tipo + búsqueda */
  var grid = $('#materials-grid');
  if (grid) {
    var cards = $$('.material-card', grid);
    var emptyBox = $('#materials-empty');
    var instaladores = $('#instaladores');
    var state = { room: 'progra', tipo: 'todos', q: '' };

    var applyFilters = function () {
      var visibles = 0;
      cards.forEach(function (card) {
        var ok = card.getAttribute('data-materia') === state.room &&
          (state.tipo === 'todos' || state.tipo === 'instalador' || card.getAttribute('data-tipo') === state.tipo) &&
          (!state.q || card.getAttribute('data-name').indexOf(state.q) !== -1);
        card.hidden = !ok;
        if (ok) { visibles++; }
      });
      emptyBox.hidden = !(visibles === 0 && state.tipo !== 'instalador');
      instaladores.hidden = state.tipo !== 'todos' && state.tipo !== 'instalador';
    };

    var bindGroup = function (selector, attr, onPick) {
      $$(selector).forEach(function (btn) {
        btn.addEventListener('click', function () {
          $$(selector).forEach(function (b) {
            b.classList.remove('active');
            b.setAttribute('aria-pressed', 'false');
          });
          btn.classList.add('active');
          btn.setAttribute('aria-pressed', 'true');
          onPick(btn.getAttribute(attr));
          applyFilters();
        });
      });
    };
    bindGroup('.subject-item', 'data-room', function (v) { state.room = v; });
    bindGroup('.chip-btn', 'data-tipo', function (v) { state.tipo = v; });

    $('.search-input').addEventListener('input', function (e) {
      state.q = e.target.value.trim().toLowerCase();
      applyFilters();
    });

    applyFilters();
  }

  /* Sandboxes: cambiar de sala */
  var sbGrid = $('#sb-grid');
  if (sbGrid) {
    var sbCards = $$('.sb-card', sbGrid);
    var sbEmpty = $('#sb-empty');
    $$('.seg-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        $$('.seg-btn').forEach(function (b) {
          b.classList.remove('active');
          b.setAttribute('aria-pressed', 'false');
        });
        btn.classList.add('active');
        btn.setAttribute('aria-pressed', 'true');
        var room = btn.getAttribute('data-room');
        var visibles = 0;
        sbCards.forEach(function (card) {
          card.hidden = card.getAttribute('data-sala') !== room;
          if (!card.hidden) { visibles++; }
        });
        sbEmpty.hidden = visibles > 0;
      });
    });
  }

  /* Unirse: estados de validación del código */
  var joinForm = $('#join-form');
  if (joinForm) {
    var input = $('#join-code');
    var status = $('#join-status');

    input.addEventListener('input', function () {
      input.value = input.value.toUpperCase().replace(/[^A-Z0-9-]/g, '');
    });

    var validate = function () {
      var code = input.value.replace(/[^A-Z0-9]/g, '');
      if (!code.length) { return; }
      if (code.length < 6) {
        status.innerHTML = '<p class="state-error">Corto: el código tiene entre 6 y 8 caracteres.</p>';
        return;
      }
      status.innerHTML = '<span class="loading-row"><span class="spinner" aria-hidden="true"></span>Buscando la sala…</span>';
      setTimeout(function () {
        if (code === 'OCI7F3K') {
          status.innerHTML =
            '<p class="state-ok">✓ ¡Listo! Entrando a Programación 5°A…</p>' +
            '<a class="btn btn-primary btn-block" href="sandboxes.html">Entrar a la sala</a>';
        } else {
          status.innerHTML = '<p class="state-error">Ese código no existe. Verificalo con tu profe.</p>';
        }
      }, 900);
    };

    joinForm.addEventListener('submit', function (e) {
      e.preventDefault();
      validate();
    });
    $('#demo-fill').addEventListener('click', function () {
      input.value = 'OCI-7F3K';
      validate();
    });
  }

  /* Detalle: streaming simulado de logs */
  var termBody = $('#term-body');
  if (termBody) {
    var LINES = [
      ['cmd', '$ ocicat run sb-42 --backend docker'],
      ['sys', '[ocicat] cache de imagen: hit (sha256:ab12…) — reusando build'],
      ['sys', '[ocicat] contenedor abc123 iniciado · mem 128MB · pids 128 · red none'],
      ['out', 'Python 3.11.6'],
      ['out', '=== Juego: Adivinanza numérica v1.2 ==='],
      ['out', 'Pensé un número entre 1 y 100. ¿Cuál es?'],
      ['out', 'Intento 1 → 50   muy alto'],
      ['out', 'Intento 2 → 25   muy alto'],
      ['out', 'Intento 3 → 12   muy bajo'],
      ['out', 'Intento 4 → 18   muy bajo'],
      ['out', 'Intento 5 → 21   ¡Correcto! Ganaste en 5 intentos.'],
      ['sys', '[ocicat] run finished · exit=0 · 3.4s'],
      ['ok', '[ocicat] registro guardado como histórico ✓']
    ];
    var GAPS = [60, 480, 340, 720, 320, 420, 640, 540, 540, 540, 760, 430, 380];
    var liveTag = $('#term-live');
    var cue = $('#autoscroll-cue');
    var auto = true;
    var i = 0;
    var clock = 14 * 3600 + 32 * 60;

    var cursor = document.createElement('span');
    cursor.className = 'cursor-blink';
    cursor.textContent = '▍';

    var pad = function (n) { return (n < 10 ? '0' : '') + n; };
    var stamp = function () {
      return pad(Math.floor(clock / 3600)) + ':' +
        pad(Math.floor((clock % 3600) / 60)) + ':' + pad(clock % 60);
    };
    var refreshCue = function () {
      if (i >= LINES.length) {
        cue.className = 'autoscroll-cue is-done';
        cue.textContent = 'fin del registro ✓';
      } else if (!auto) {
        cue.className = 'autoscroll-cue is-paused';
        cue.textContent = 'pausado — ir al final ↓';
      } else {
        cue.className = 'autoscroll-cue';
        cue.textContent = 'en vivo · bajando solo ↓';
      }
    };

    var appendLine = function () {
      var row = document.createElement('div');
      row.className = 'log-line';
      var time = document.createElement('span');
      time.className = 'log-time';
      time.textContent = stamp();
      var text = document.createElement('span');
      text.className = 'log-' + LINES[i][0];
      text.textContent = LINES[i][1];
      row.appendChild(time);
      row.appendChild(text);
      termBody.insertBefore(row, cursor.parentNode === termBody ? cursor : null);
      if (auto) { termBody.scrollTop = termBody.scrollHeight; }
    };

    var tick = function () {
      if (i >= LINES.length) {
        liveTag.innerHTML = '<span class="dot"></span>terminado · exit=0';
        refreshCue();
        return;
      }
      clock += Math.round(GAPS[i] / 1000);
      appendLine();
      i++;
      refreshCue();
      setTimeout(tick, GAPS[i - 1]);
    };

    termBody.appendChild(cursor);
    termBody.addEventListener('scroll', function () {
      auto = termBody.scrollTop + termBody.clientHeight >= termBody.scrollHeight - 30;
      refreshCue();
    });
    cue.addEventListener('click', function () {
      termBody.scrollTop = termBody.scrollHeight;
      auto = true;
      refreshCue();
    });

    setTimeout(tick, 600);
  }
})();
