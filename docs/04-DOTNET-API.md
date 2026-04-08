# Siigo - API de DLLs .NET

## Contexto

Siigo incluye dos DLLs .NET compiladas desde COBOL con Micro Focus Visual COBOL:
- `SIIGOCN.dll` — Business Objects (lógica de negocio)
- `SIIGOCV.dll` — DTOs (Data Transfer Objects) y Session

Estas DLLs son **.NET Framework 4.8, x86** y dependen de `MicroFocus.COBOL.Runtime.dll`.

## LIMITACIÓN CRÍTICA

**Los DTOs NO exponen datos como propiedades .NET.** Todos los DTOs solo tienen una propiedad visible: `_MF_CONTROL` (de tipo `MicroFocus.COBOL.Program.ObjectControl`). Los datos reales están almacenados en WORKING-STORAGE de COBOL, que es memoria interna no accesible desde .NET estándar.

Esto significa que las DLLs sirven para:
- Inicialización del sistema
- Gestión de sesión
- Información del sistema
- **NO sirven para leer datos de negocio** (terceros, productos, movimientos)

Para leer datos de negocio, hay que leer los archivos ISAM directamente (ver `02-ISAM-FORMAT.md`).

---

## SIIGOCN.dll — Business Objects

### SIIGOLauncher (namespace: SIIGOCN.OTHERS)

La clase principal para inicializar Siigo.

#### Métodos Estáticos
```csharp
// Carga configuración de la empresa activa
static void LoadConfigEmp()

// Carga lista de empresas (retorna objeto COBOL, no útil desde .NET externo)
static object LoadEmpresas()

// Carga el menú del sistema
static object LoadMenu()

// Valida usuario y contraseña
// Retorna "S" (válido) o "N" (inválido)
// NOTA: Siempre retorna "N" con datos demo — las contraseñas están cifradas
static string ValidarUsuarioPassword(string usuario, string password, string gerente)

// Versión nube del login
static string ValidarUsuarioPasswordNube(string usuario, string gerente, string password)

// Ejecuta un programa COBOL (.gnt) por nombre
// ¡NO FUNCIONA! Requiere NativeLauncher.dll que es nativa (no .NET)
static string ExecuteCall(string program, string param, string niveles)

// Ejecuta un método/evento del sistema
static string ExecuteMethod(string evento)

// Bloqueo del sistema
static void SIIGOLock()

// Gestión de favoritos
static void addFavorite(string param)
static void DelFavorite(string param)
static void UpFavorite(string param)
static void DownFavorite(string param)
static void TopFavorite(string param)
static void BottomFavorite(string param)

// Carga favoritos y comprobantes activos
static object LoadFavoritesAct()
static object LoadComprobantesAct()

// Carga URL por código
static string LoadURL(string codigo)
```

#### Métodos de Instancia
```csharp
// Carga la ruta del directorio de datos desde FILEPATH.TXT
// Retorna: "C:\SIIWI02\" (con trailing backslash)
// IMPORTANTE: busca FILEPATH.TXT en el working directory actual
string LoadFilePath()
```

---

### BOTBL_Q_1 — Empresa (namespace: SIIGOCN.SIIGOBO)
```csharp
// Forzar el número de empresa a usar
void ForceNumEnterprise(int numEmpresa)

// Cargar datos de la empresa
// Retorna DTOTBL_Q_1 (pero los datos no son accesibles como propiedades .NET)
DTOTBL_Q_1 Cargar()

// Inicializar con un DTO existente
void Inicializar(DTOTBL_Q_1 dto)
```

### BOTBL_K_1 — Usuarios (namespace: SIIGOCN.SIIGOBO)
```csharp
void ForceNumEnterprise(int numEmpresa)

// Cargar datos de un usuario
// usuario: "ADMON", gerente: "" o "S"
DTOTBL_K_1 Cargar(string usuario, string gerente)

// Desencriptar un valor cifrado
string DesEncripta(string crypte)

void Inicializar(DTOTBL_K_1 dto)

// NO tiene métodos de escritura (Add, Del, Update)
```

### BOTBL_CP_11 — Plan de Cuentas (namespace: SIIGOCN.SIIGOBO)
```csharp
void ForceNumEnterprise(int numEmpresa)
object Cargar()

// Obtener nombre de una cuenta por código
// NOTA: El parámetro es tipo MicroFocus.COBOL.Program.Reference, NO string
// No se puede llamar directamente con un string
string ObtenerNombre(Reference codigo)

void Inicializar(object dto)
```

### BOMEN — Menú del Sistema (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarPorLlave1(string llave)
object CargarPorLlave5(string llave)
object CargarTodos()  // Carga todos los menús
void Inicializar(object dto)
```

### BOMENMAEFA — Favoritos (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarPorLlave1(string llave)
object CargarTodos()
object CargarPorUsuario()
void Add(object dto)     // ¡Tiene método de escritura!
void Del(string key, string user)
void Up()
void Down()
void Inicializar(object dto)
```

### BOMENMAECP — Comprobantes (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarTodos()
object CargarPorUsuario()
void Inicializar(object dto)
```

### BORUTA — Rutas/URLs (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarPorLlave1(string llave)
string ObtenerURLporCodigo(string codigo)
void Inicializar(object dto)
```

### BOAGENDA — Agenda (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarPorLlave1(string llave)
object CargarPorUsuario()
```

### BOLLA — Llamados (namespace: SIIGOCN.SIIGOBO)
```csharp
object CargarLlamadoDesdePath(string pathFile)
```

---

## SIIGOCV.dll — DTOs y Session

### SIIGOSession (namespace: SIIGOCV.SESSION)

Gestión de sesión del usuario. **Todos los métodos son estáticos.**

```csharp
// Sesión completa como cadena
static string getLLA()
static void setLLA(ref string lla)

// Permisos del usuario
static string getPermisos()

// DTO de empresa en sesión
static DTOTBL_Q_1 getDTOTBLQ()
static void setDTOTBLQ(DTOTBL_Q_1 dto)

// DTO de usuario en sesión
static DTOTBL_K_1 getDTOTBLK()
static void setDTOTBLK(DTOTBL_K_1 dto)

// Buscar/actualizar claves de sesión
static string FindKey(string key)
static void UpdateKey(string key, string value)

// Tipo de Siigo (país)
static string TypeSIIGO()       // Retorna: "ES" (España/Colombia)
static string TypeSIIGOString() // Retorna tipo como string

// Sesión nube
static void LoadLLAFromString(string key)
static void updateKeyCloud(string key, string value)
static string findKeyCloud(string key)
static void LoadSessionNubefromString(string data)

// Sesión POS y áreas
static string ObtainStringFromSessionPOS()
static string ObtainStringFromSessionNube()
static string ObtainStringFromSessionArea3()
static string ObtainStringFromSessionArea4()
static void savePOSFromString(string data)
static void saveArea3FromString(string data)
static void saveArea4FromString(string data)

// Nómina
static void updateKeyNomina(string key, string value)
static string findKeyNomina(string key)
```

**Claves de sesión conocidas:**
```
EMPRESA    — Nombre de la empresa
USUARIO    — Código de usuario (ej: "ADMON")
CLAVE      — Contraseña
SERIAL     — Serial del producto
FILEPATH   — Ruta de datos
NUMEMPRESA — Número de empresa
NOMBRE     — Nombre del usuario
GERENTE    — "S"/"N" si es gerente
ADMIN      — "S"/"N" si es administrador
PERMISO    — Permisos
TIPO       — Tipo
VERSION    — Versión
PAIS       — País
MODULO     — Módulo activo
```

### SIIGOFiles (namespace: SIIGOCV.SESSION)

Utilidades de archivos. **Todos los métodos son estáticos.**

```csharp
static string ReadFECHAACT()           // Retorna: "[2026/02/17]"
static string ReadSIIGOCFG()
static void WriteSIIGOCFG(int size)
static void CreateDir(string name)
static void ConfigFilesSIIGO()
static void CopyFiles()
static void CopyFilesProgData()
static void CopyFilesDataProg()
static void CopyFilesFromDir()
static string ReadFECHAACTActualizador(string path)
```

### DTOs Disponibles (namespace: SIIGOCV.SIIGODTO)

Todos solo exponen `_MF_CONTROL` como propiedad .NET:

| DTO | Usado por |
|-----|-----------|
| `DTOAGENDA` | BOAGENDA |
| `DTOMEN` | BOMEN |
| `DTOMENMAECP` | BOMENMAECP |
| `DTOMENMAEFA` | BOMENMAEFA |
| `DTOTBL_CP_11` | BOTBL_CP_11 |
| `DTOTBL_K_1` | BOTBL_K_1 |
| `DTOTBL_Q_1` | BOTBL_Q_1 |
| `DTOTRUTAS` | BORUTA |

---

## Código Funcional Completo

### Proyecto .csproj
```xml
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net48</TargetFramework>
    <Platform>x86</Platform>
    <LangVersion>latest</LangVersion>
  </PropertyGroup>
  <ItemGroup>
    <Reference Include="SIIGOCN">
      <HintPath>C:\Siigo\SIIGOCN.dll</HintPath>
      <Private>true</Private>
    </Reference>
    <Reference Include="SIIGOCV">
      <HintPath>C:\Siigo\SIIGOCV.dll</HintPath>
      <Private>true</Private>
    </Reference>
    <Reference Include="MicroFocus.COBOL.Runtime">
      <HintPath>C:\Siigo\MicroFocus.COBOL.Runtime.dll</HintPath>
      <Private>true</Private>
    </Reference>
    <Reference Include="System.Windows.Forms" />
  </ItemGroup>
</Project>
```

### Inicialización
```csharp
using SIIGOCN.OTHERS;
using SIIGOCN.SIIGOBO;
using SIIGOCV.SIIGODTO;
using SIIGOCV.SESSION;

// IMPORTANTE: ejecutar desde C:\Siigo\ (busca FILEPATH.TXT en working dir)

// 1. Cargar ruta de datos
var launcher = new SIIGOLauncher();
string filePath = launcher.LoadFilePath();  // "C:\SIIWI02\"

// 2. Cargar configuración
SIIGOLauncher.LoadConfigEmp();

// 3. Configurar empresa en sesión
var boEmp = new BOTBL_Q_1();
boEmp.ForceNumEnterprise(1);
var dtoEmp = boEmp.Cargar();
SIIGOSession.setDTOTBLQ(dtoEmp);

// 4. Configurar usuario en sesión
var boUser = new BOTBL_K_1();
boUser.ForceNumEnterprise(1);
var dtoUser = boUser.Cargar("ADMON", "");
SIIGOSession.setDTOTBLK(dtoUser);

// 5. Establecer claves de sesión
SIIGOSession.UpdateKey("USUARIO", "ADMON");
SIIGOSession.UpdateKey("GERENTE", "S");
SIIGOSession.UpdateKey("ADMIN", "S");

// 6. Verificar
string tipo = SIIGOSession.TypeSIIGO();          // "ES"
string fecha = SIIGOFiles.ReadFECHAACT();         // "[2026/02/17]"
```

### Compilar y Ejecutar
```bash
# Compilar
cd /c/Users/lordmacu/siigo/SiigoExplorer
dotnet build -c Debug

# Ejecutar (DEBE ser desde C:\Siigo\)
cd /c/Siigo
/c/Users/lordmacu/siigo/SiigoExplorer/bin/x86/Debug/net48/SiigoExplorer.exe
```

---

## Business Objects Nativos (GNT) — NO Accesibles

Los verdaderos Business Objects que leen/escriben datos son programas COBOL nativos (.gnt) que NO se pueden invocar desde .NET externo:

| Programa | Función |
|----------|---------|
| BOINV.gnt | Inventario |
| BOMOV.gnt | Movimientos |
| BOMAE.gnt | Maestros |
| BOCLINT.gnt | Clientes |
| BOCOS.gnt | Costos |
| BODET.gnt | Detalle |
| BOPED.gnt | Pedidos |
| BOFELE.gnt | Facturación electrónica |
| BOTBL-*.gnt | Tablas de configuración (A, B, CP, EX, H, I, K, O, P, Q, etc.) |

Estos requieren `NativeLauncher.dll` (DLL nativa C/C++, no .NET) para ejecutarse, lo cual no es posible desde un programa .NET externo.
