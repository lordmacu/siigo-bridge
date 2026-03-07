# Siigo - Instalación, Configuración y Problemas Resueltos

## Instalación de Siigo Pyme

### Requisitos
- Windows 10/11 (x86 o x64)
- .NET Framework 4.8
- Visual C++ Redistributable (x86 y x64)
- Micro Focus COBOL Server 9.0 (incluido en el instalador de Siigo)

### Directorios después de instalar
```
C:\Siigo\                    → Programas y configuración
C:\Siigo\MFServer90\         → Instaladores Micro Focus COBOL
C:\DEMOS01\                  → Datos de la empresa demo
C:\ProgramData\Micro Focus\  → Licencias y configuración del runtime
```

### Usuarios por defecto
- **ADMON** / contraseña por configurar al primer inicio
- Tipo: Administrador / Gerente

---

## Problema 1: Error 245 — "There are no valid product licenses"

### Síntoma
Al abrir Siigo aparece:
```
Execution error: file ''
error code: 245, pc=0, call=1, seg=0
245  There are no valid product licenses
```

### Causa
El runtime de Micro Focus COBOL no encuentra una licencia válida. El instalador de Siigo incluye `C:\Siigo\MFServer90\cs_90.exe` que instala el runtime, pero a veces la licencia no se registra correctamente.

### Diagnóstico
```bash
# Verificar si el servicio de licencias está corriendo
"C:\Program Files (x86)\Micro Focus\Licensing\mfcesdchk.exe"
# Debe decir: "CES daemon running, version 10000.3.19302"

# Verificar licencias instaladas
"C:\Program Files (x86)\Micro Focus\Licensing\MFLicenseAdmin.exe" -list
# Si no muestra licencias → ese es el problema

# Decodificar la licencia para verificar que es válida
"C:\Program Files (x86)\Micro Focus\Licensing\mflsdecode.exe" -s "C:\Siigo\MFServer90\License90.mflic"
# Debe mostrar: "COBOL-Server-(Production)(PA)", "SIIGO S.A.", "no expiration"
```

### Solución

**Paso 1:** Copiar la licencia al archivo correcto.

La licencia de Siigo es tipo **NETWORK** (no STANDALONE), por lo tanto debe ir en `lservrc.net` (no `lservrc.stn`).

```
Copiar el contenido de:
  C:\Siigo\MFServer90\License90.mflic
A:
  C:\ProgramData\Micro Focus\lservrc.net
```

El contenido es una línea larga que empieza con:
```
10 SolarNativeRuntimeDeploy 1.0 LONG NORMAL NETWORK ADD INFINITE_KEYS ...
```

**Paso 2:** Editar `C:\ProgramData\Micro Focus\ces.ini`:

Cambiar:
```ini
lsfile=C:\ProgramData\Micro Focus\lservrc.stn
```
Por:
```ini
lsfile=C:\ProgramData\Micro Focus\lservrc.net
```

**Paso 3:** Reiniciar el servicio (requiere CMD como administrador):
```
net stop "Micro Focus CES daemon"
net start "Micro Focus CES daemon"
```

### Detalles de la licencia
```
Producto:     COBOL Server (Production)(PA)
Serial:       600000406891
Tipo:         Normal Network
Feature:      SolarNativeRuntimeDeploy v1.0
Usuarios:     Ilimitados
Expiración:   Nunca
Cliente:      SIIGO S.A.
Plataforma:   Win XP/Vista/7/8.1/Server 2003/2008/2012
Generada:     2015-12-07
```

### Servicios de Windows involucrados
```
"Micro Focus CES daemon"           → Servidor de licencias principal
"Sentinel RMS License Manager"     → Licencias legacy (el log está en lservrc.log)
"Micro Focus AutoPass Daemon"      → AutoPass licensing
"Micro Focus Directory Server"     → Directorio
"Micro Focus Enterprise Server Common Web Administration"
```

### Archivos de licencia
```
C:\Siigo\MFServer90\License90.mflic          → Licencia original
C:\ProgramData\Micro Focus\lservrc.net       → Licencias NETWORK (aquí va)
C:\ProgramData\Micro Focus\lservrc.stn       → Licencias STANDALONE
C:\ProgramData\Micro Focus\ces.ini           → Configuración del CES daemon
C:\ProgramData\Micro Focus\lservrc.log       → Log del license manager
C:\ProgramData\Micro Focus\LicFile.txt       → Lista XML de licencias (vacío por defecto)
C:\ProgramData\Micro Focus\CommuterLicFile.xml → Licencias commuter
```

### Por qué MFLicenseAdmin -install no funciona
`MFLicenseAdmin.exe -install` solo acepta licencias STANDALONE. Las licencias NETWORK se rechazan con el mensaje:
```
Non-STANDALONE license ignored: 10 SolarNativeRuntimeDeploy...
```
Por eso hay que copiar manualmente al archivo `lservrc.net`.

### Instaladores incluidos en MFServer90
```
cs_90.exe                → Micro Focus COBOL Server 9.0 (521MB)
cs_90_pu07_350133.exe    → Parche/actualización 7 (376MB)
VC_redist_x64.exe        → Visual C++ Redistributable x64 (25MB)
VC_redist_x86.exe        → Visual C++ Redistributable x86 (13MB)
License90.mflic          → Archivo de licencia
```

Si el runtime no está instalado, ejecutar `cs_90.exe` como administrador, luego `cs_90_pu07_350133.exe`.

---

## Problema 2: DLLs .NET no encontradas al ejecutar

### Síntoma
```
System.IO.FileNotFoundException: No se puede cargar el archivo o ensamblado 'SIIGOCV'
```

### Causa
Las DLLs referenciadas (SIIGOCN.dll, SIIGOCV.dll, MicroFocus.COBOL.Runtime.dll) no se copian al directorio de salida del build.

### Solución
En el archivo `.csproj`, asegurar que `<Private>true</Private>` está en cada referencia:
```xml
<Reference Include="SIIGOCN">
  <HintPath>C:\Siigo\SIIGOCN.dll</HintPath>
  <Private>true</Private>  <!-- CRÍTICO: debe ser true -->
</Reference>
```

---

## Problema 3: FILEPATH.TXT no encontrado

### Síntoma
Error al llamar `LoadFilePath()` — no encuentra el archivo FILEPATH.TXT.

### Causa
Las DLLs de Siigo buscan `FILEPATH.TXT` en el **working directory actual**, no en `C:\Siigo\`.

### Solución
Ejecutar el programa siempre desde `C:\Siigo\`:
```bash
cd /c/Siigo && /ruta/completa/al/SiigoExplorer.exe
```

---

## Problema 4: ExecuteCall falla con NativeLauncher.dll

### Síntoma
```
System.IO.FileNotFoundException: No se puede cargar el archivo o ensamblado 'NativeLauncher'
```

### Causa
`NativeLauncher.dll` es una DLL nativa (C/C++), no un assembly .NET. No se puede cargar con `Assembly.Load` ni referenciarlo desde un proyecto .NET.

### Solución
**No tiene solución.** `ExecuteCall` no es viable para ejecutar programas COBOL desde .NET externo. El enfoque alternativo es leer los archivos ISAM directamente.

---

## Problema 5: Archivo ISAM bloqueado

### Síntoma
```
El proceso no puede obtener acceso al archivo 'C:\DEMOS01\Z003' porque está siendo utilizado por otro proceso.
```

### Causa
Siigo tiene el archivo abierto con bloqueo exclusivo.

### Solución
Abrir los archivos con `FileShare.ReadWrite`:
```csharp
using (var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
{
    // leer datos
}
```

**Nota:** Algunos archivos como Z003 (usuarios) pueden estar bloqueados incluso con `FileShare.ReadWrite` mientras Siigo está activo. Para esos archivos, leer cuando Siigo esté cerrado o aceptar que no estarán disponibles.

---

## Problema 6: CS1503 — string vs Reference

### Síntoma
```
error CS1503: Argumento 1: no se puede convertir de 'string' a 'MicroFocus.COBOL.Program.Reference'
```

### Causa
Algunos métodos de las DLLs COBOL esperan `MicroFocus.COBOL.Program.Reference` en lugar de `string`.

### Solución
Omitir esos métodos o crear un wrapper que convierta. Por ahora se omiten ya que los datos se leen directamente de ISAM.

---

## Configuración del Proyecto SiigoExplorer

### Ubicación
```
C:\Users\lordmacu\siigo\SiigoExplorer\
├── SiigoExplorer.csproj
├── Program.cs
├── bin\x86\Debug\net48\
│   ├── SiigoExplorer.exe
│   ├── SIIGOCN.dll
│   ├── SIIGOCV.dll
│   └── MicroFocus.COBOL.Runtime.dll
└── obj\
```

### Compilar
```bash
cd /c/Users/lordmacu/siigo/SiigoExplorer
dotnet build -c Debug
```

### Ejecutar
```bash
cd /c/Siigo
/c/Users/lordmacu/siigo/SiigoExplorer/bin/x86/Debug/net48/SiigoExplorer.exe
```

**Siempre ejecutar desde `C:\Siigo\`** para que encuentre FILEPATH.TXT.
