# Preparar una publicación en GitHub

Esta guía prepara el repositorio para que su propietario lo publique. No publica nada por sí sola.

## 1. Revisa el contenido

- Conserva `LICENSE`, `NOTICE.md` y los avisos de autoría existentes. El proyecto es GPL-3.0 y parte de `thirteenth-order/kh3-save-editor` v0.2.1.
- Lee [Privacidad y seguridad](PRIVACIDAD.md) y verifica que las afirmaciones describan el comportamiento real de la versión que vas a subir.
- Ejecuta las pruebas y vuelve a compilar el ejecutable como indica [Compilar y probar](COMPILAR_Y_PROBAR.md). El `.exe` que exista en una computadora local no debe darse por verificado automáticamente.
- Usa solo fixtures sintéticos para pruebas, capturas o grabaciones.
- Comprueba `git status --short` y revisa uno por uno los archivos que planeas incluir. `.gitignore` excluye ejecutables, guardados, copias `.bak.*`, archivos zip, volcados JSON y rutas de trabajo habituales; aun así, inspecciona la lista antes del commit.

No uses `git add .` a ciegas. No incluyas archivos personales, volcados, identificadores de cuenta, salidas de herramientas, binarios locales ni carpetas de respaldo.

## 2. Crea tu repositorio

En GitHub, crea un repositorio vacío bajo tu cuenta. No agregues otro README, licencia o `.gitignore` desde el asistente de creación: este árbol ya contiene esos archivos. Si prefieres publicar desde un fork del repositorio upstream, primero adapta los pasos de rama y push a la rama que ya exista en ese fork.

El checkout local parte del tag upstream `v0.2.1` y puede tener el remoto `origin` apuntando al proyecto original. Reemplaza `TU-USUARIO` y confirma la URL antes de ejecutar los comandos:

```powershell
git switch -c main
git remote rename origin upstream
git remote add origin https://github.com/TU-USUARIO/kh3-save-editor.git
git remote -v
```

Si ya existe una rama local `main`, elige un nombre de rama apropiado en vez de ejecutar `git switch -c main` sin revisar.

## 3. Revisa, confirma y sube los cambios

Agrega únicamente el material revisado. Por ejemplo:

```powershell
git add README.md TRANSFERENCIA.md NOTICE.md .gitignore "Iniciar transferencia.bat" cmd/kh3save/main.go internal/transfer docs media/epic-steam-cover.svg media/flujo-transferencia.svg .github
git diff --cached --stat
git diff --cached --name-only
git diff --cached
```

Si la lista contiene guardados, datos personales, `kh3save.exe` o archivos que no reconoces, quítalos del área de preparación y vuelve a revisar. Asegúrate de que `LICENSE` y el código fuente que corresponde a los cambios GPL también formen parte del repositorio.

Cuando la revisión esté limpia:

```powershell
git commit -m "Add Spanish Epic-to-Steam transfer assistant"
git push -u origin main
```

Después de subirlo, comprueba en la página del repositorio que README, licencia, avisos y diagramas se muestran bien.

## 4. Publica binarios como Release

No guardes ejecutables compilados localmente en el historial Git. Tras revisar el código y compilar una versión limpia, crea una GitHub **Release** etiquetada; adjunta el ejecutable de Windows y su SHA-256. Incluye el código fuente correspondiente a esa misma versión, las instrucciones de compilación y las notas GPL-3.0. No distribuyas datos de usuario dentro de los archivos de la Release.

## 5. Imágenes

Los diagramas de `media/epic-steam-cover.svg` y `media/flujo-transferencia.svg` son ilustraciones vectoriales originales para el README. No usan logos ni arte del juego. Si los sustituyes, conserva esa regla y añade texto alternativo descriptivo.
