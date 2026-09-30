# Compilar y probar

## Requisitos

- Windows para el iniciador `.bat`, el selector nativo de carpetas y la prueba completa de la interfaz local.
- Go **1.26 o posterior**, según `go.mod`.
- Node.js solo si quieres comprobar la sintaxis del JavaScript.

## Compilar

Desde la raíz del repositorio, en PowerShell:

```powershell
go build -o kh3save.exe ./cmd/kh3save
```

Después, ejecuta `Iniciar transferencia.bat`. La ventana de consola debe permanecer abierta mientras se usa el navegador. Para iniciar la interfaz general del editor upstream, ejecuta `kh3save.exe` sin argumentos.

## Comprobaciones con datos sintéticos

Las pruebas Go generan datos de prueba localmente y no requieren guardados personales:

```powershell
go test ./...
```

Comprobación opcional del JavaScript:

```powershell
node --check .\internal\transfer\assets\app.js
```

Al probar manualmente el flujo, usa únicamente fixtures generados para desarrollo. Revisa que el resumen enumere cada archivo, que los reemplazos requieran confirmación y que la carpeta de respaldo sea visible. No subas guardados reales a los issues, a pull requests ni a servicios de CI.

## Crear un ejecutable de distribución

Compila desde el código fuente después de revisar los cambios y ejecutar las pruebas:

```powershell
go test ./...
go build -trimpath -ldflags "-s -w" -o kh3save.exe ./cmd/kh3save
Get-FileHash .\kh3save.exe -Algorithm SHA256
```

Publica el SHA-256 junto al archivo en una **Release**. Antes de adjuntar binarios, valida el flujo con fixtures sintéticos en un equipo Windows limpio. No incluyas guardados, volcados, SteamID64 de personas ni archivos de respaldo.
