# KH III + Re Mind: Epic Games Store a Steam

Asistente local en español para Windows. Abre el navegador en `127.0.0.1` y
usa directamente el código de formato de guardado de
[`kh3-save-editor`](https://github.com/thirteenth-order/kh3-save-editor),
versión upstream **v0.2.1**. Esa versión ya implementa la detección de Epic,
el contenedor de Steam, el rekey por cuenta y las comprobaciones CRC32/MD5.
Este asistente agrega el flujo de migración y no vuelve a implementar el
formato ni la clave. Se conservan `LICENSE` y `NOTICE.md` del proyecto GPL-3.0.

## Iniciar

1. Cierra KINGDOM HEARTS III.
2. Haz doble clic en **Iniciar transferencia.bat**. Se abrirá la interfaz en el
   navegador. Mantén abierta la ventana de consola mientras la uses; cerrarla
   apaga el servidor local.
3. En el paso 1, pulsa **Abrir explorador de archivos** para seleccionar varios
   `KHIII_slot*.bin` y, si está presente, `KHIII_system.bin`. También puedes
   arrastrarlos a la zona de selección o elegir la carpeta Epic detectada / la
   carpeta `SaveGames\kh3sv2\data`. Los archivos seleccionados se procesan en
   memoria y se envían solo por loopback al asistente de esta computadora.
4. En el paso 2, elige la carpeta de datos de Steam, la carpeta de cuenta
   `Steam\<SteamID64>` o la carpeta `Steam`. Si el identificador no se detecta,
   ingrésalo tal como aparece en el nombre de la carpeta Steam: **17 dígitos**,
   normalmente empieza con `7656119`. No ingreses el `accountid` de
   `steam_autocloud.vdf`.
5. Revisa la lista, el destino y los reemplazos. Confirma los reemplazos si los
   hay. La conversión se prepara y valida primero; luego debes confirmar la
   instalación.
6. Al terminar, revisa el destino y la ruta de copias de seguridad que muestra
   la interfaz. Puedes volver a activar Steam Cloud después de comprobar la
   partida.

El asistente detecta y convierte los `KHIII_slot*.bin` y `KHIII_system.bin`
que encuentra y reconoce como contenedores Epic válidos. Conserva sus nombres.
No acepta guardados de KH 1.5 + 2.5, contenedores de consola ni archivos cuyo
formato o integridad no pueda validar. Un error detiene la operación antes de
instalar el conjunto.

## Copias de seguridad y restauración

Cada instalación crea una carpeta visible bajo `Copias de seguridad`, dentro
del destino elegido. `Epic` contiene copias exactas de los originales de Epic;
`Steam reemplazado` contiene copias exactas de los archivos Steam que se
sobrescribieron. La herramienta no modifica los originales Epic ni mantiene
copias ocultas. La carpeta temporal de preparación se elimina al terminar.

Para restaurar, cierra el juego y Steam Cloud. Copia los archivos de `Epic` de
vuelta a su carpeta Epic si quieres restaurar ese origen, o copia los de
`Steam reemplazado` de vuelta al destino Steam. Si Steam no tenía archivos que
reemplazar, elimina del destino los `KHIII_*.bin` instalados para deshacer la
migración. Luego vuelve a activar Steam Cloud cuando hayas terminado.

Si cierras la herramienta antes de confirmar la instalación, puede quedar la
carpeta visible `Preparacion temporal KH3-*` dentro del destino. Contiene solo
la salida preparada; elimínala si cancelaste la operación.

## Límites

La aplicación se enlaza solo a `127.0.0.1`; la conversión ocurre en esta
computadora y no envía partidas ni credenciales a internet. No solicita cuenta
ni contraseña de Epic, contraseña/cookies de Steam, ni claves manuales. El
formato y los checksums verificados indican que los archivos se pudieron
convertir y releer con la implementación upstream; no garantizan que el juego
los acepte. Comprueba dentro del juego que aparecen los espacios transferidos.
El progreso transferido no garantiza logros de Steam.

## Compilar desde el código fuente

Se incluye `kh3save.exe` para iniciar directamente en este equipo. Si necesitas
reconstruirlo, usa Go 1.26 o posterior desde esta carpeta:

```powershell
go build -o kh3save.exe ./cmd/kh3save
```

La interfaz de edición upstream sigue disponible ejecutando `kh3save.exe` sin
argumentos; el iniciador abre el asistente de migración (`transfer`).
