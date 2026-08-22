from sqlalchemy import Column, Integer, String, ForeignKey
from sqlalchemy.ext.declarative import declarative_base

Base = declarative_base()

class Alumno(Base):
    __tablename__ = "alumnos"
    id = Column(Integer, primary_key=True)
    nombre = Column(String)

class Material(Base):
    __tablename__ = "materiales"
    id = Column(Integer, primary_key=True)
    titulo = Column(String)

class Proyecto(Base):
    __tablename__ = "proyectos"
    id = Column(Integer, primary_key=True)
    nombre = Column(String)
    alumno_id = Column(Integer, ForeignKey("alumnos.id"))
