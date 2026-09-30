# Privacidad, seguridad y límites

## Qué ocurre con los archivos

La interfaz se sirve desde un proceso local que escucha en `127.0.0.1` con un puerto aleatorio y un token temporal. Los bytes seleccionados en el navegador se envían al mismo equipo por loopback para validarlos y preparar la conversión. El programa no tiene una función de red para enviar archivos a servicios externos y no recopila telemetría.

Los archivos seleccionados se guardan en memoria durante la sesión; el asistente no crea una copia de carga oculta en disco. Al instalar, crea copias persistentes visibles: originales de Epic y, si se reemplazaron, originales de Steam. También prepara primero la salida en una carpeta temporal visible y vuelve a leerla antes de instalar.

La dirección local no es una web pública. No publiques un proxy, túnel o puerto hacia internet para esta herramienta. Cualquiera que obtuviera acceso al proceso local podría interactuar con los archivos que el usuario seleccione.

## Datos que no se solicitan

No se pide la contraseña de Epic o Steam, cookies, tokens de sesión ni credenciales. El SteamID64 se lee de la carpeta de destino cuando es posible o se introduce manualmente para derivar la clave de conversión conforme a la implementación upstream. No lo compartas en capturas o informes públicos.

## Alcance de la validación

Se conservan los nombres de los archivos compatibles de KH III para los espacios y archivos auxiliares seleccionados. La implementación upstream comprueba el contenedor y sus datos de integridad y la herramienta vuelve a abrir el resultado como guardado de Steam. Eso no demuestra que el juego acepte la partida ni que Steam conceda logros.

No se aceptan archivos identificados como guardados de consola, formatos de KINGDOM HEARTS HD 1.5 + 2.5 ReMIX ni archivos que no pasen las comprobaciones disponibles. Si el formato o la clave no se pueden confirmar, la herramienta debe detener la operación.

## Restaurar una copia

1. Cierra el juego y desactiva Steam Cloud temporalmente.
2. Restaura los archivos de la subcarpeta `Epic` a la carpeta original de Epic, o los de `Steam reemplazado` a la carpeta de Steam, según lo que quieras recuperar.
3. Si no había archivos Steam reemplazados y quieres deshacer la instalación, quita del destino Steam los archivos `KHIII_*.bin` que instaló la herramienta.
4. Comprueba el resultado antes de volver a activar Steam Cloud.

Consulta [TRANSFERENCIA.md](../TRANSFERENCIA.md) para las rutas completas y las instrucciones paso a paso.

## Reportar un problema

Describe el sistema operativo, la versión del proyecto, el nombre del archivo (sin adjuntarlo), el paso que falla y el texto exacto del error. No publiques partidas, copias de seguridad, SteamID64, rutas de usuario, cookies ni credenciales. Usa únicamente datos sintéticos si hace falta reproducir un caso.
