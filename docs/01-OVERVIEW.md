# Siigo Pyme - Ingeniería Inversa Completa

## ¿Qué es Siigo?

Siigo Pyme es un software de contabilidad y ERP colombiano usado por miles de empresas. Maneja contabilidad, facturación, inventario, cartera, nómina, activos fijos, y más.

## ¿Qué estamos haciendo?

Ingeniería inversa para construir un **middleware** que lea datos de Siigo y los envíe a una plataforma web de órdenes de compra. Siigo NO tiene una API pública, así que tuvimos que descifrar cómo almacena los datos internamente.

## ¿Qué descubrimos?

### La sorpresa: Siigo NO usa una base de datos SQL

Siigo está escrito en **COBOL** (sí, COBOL, el lenguaje de los años 60) compilado con **Micro Focus COBOL Server 9.0**. Los datos se almacenan en archivos **ISAM** (Indexed Sequential Access Method) — un formato binario propietario donde cada "tabla" son 2 archivos: uno de datos y uno de índices.

### Stack Tecnológico Completo

```
┌─────────────────────────────────────────────────┐
│  Interfaz de Usuario (Windows Forms / COBOL UI) │
├─────────────────────────────────────────────────┤
│  DLLs .NET (wrappers limitados)                 │
│  - SIIGOCN.dll (Business Objects)               │
│  - SIIGOCV.dll (DTOs, Session)                  │
├─────────────────────────────────────────────────┤
│  Programas COBOL nativos (.gnt)                 │
│  - ~3205 programas compilados                   │
│  - BOINV, BOMOV, BOMAE, BOCLINT, etc.          │
├─────────────────────────────────────────────────┤
│  Micro Focus COBOL Runtime 9.0                  │
│  - Extended File Handler (XFH)                  │
│  - IDXFORMAT=8                                  │
├─────────────────────────────────────────────────┤
│  Archivos ISAM (datos binarios)                 │
│  - Z17, Z06, Z49, C03, etc.                    │
│  - Cada archivo = datos + .idx (índice)         │
└─────────────────────────────────────────────────┘
```

### Rutas en el sistema

| Ruta | Qué contiene |
|------|-------------|
| `C:\Siigo\` | Instalación: programas .gnt, DLLs, configuración |
| `C:\DEMOS01\` | Datos de la empresa demo (~957 archivos) |
| `C:\Siigo\FILEPATH.TXT` | Contiene la ruta al directorio de datos activo (ej: `C:\DEMOS01\`) |
| `C:\Siigo\EXTFH.CFG` | Configuración del File Handler ISAM |
| `C:\Siigo\COBCONFIG.CFG` | Config COBOL: `set no_mfredir=TRUE` |
| `C:\Siigo\SIIGO.CFG` | Versión (91), URLs de contrato |
| `C:\ProgramData\Micro Focus\` | Licencias y servicios Micro Focus |

### Tres caminos que investigamos

1. **DLLs .NET (SIIGOCN.dll, SIIGOCV.dll)** → Tienen Business Objects y DTOs pero los DTOs NO exponen datos como propiedades .NET. Los datos quedan en memoria COBOL interna. **DESCARTADO para lectura de datos**.

2. **ExecuteCall (invocar programas COBOL)** → Requiere `NativeLauncher.dll` que es una DLL nativa (C/C++), no se puede cargar desde .NET externo. **DESCARTADO**.

3. **Leer archivos ISAM directamente** → **FUNCIONA**. Pudimos descifrar el formato binario y extraer datos reales: nombres de clientes, productos, movimientos contables, etc.

## Documentos en este directorio

| Archivo | Contenido |
|---------|-----------|
| `01-OVERVIEW.md` | Este documento - visión general |
| `02-ISAM-FORMAT.md` | Formato binario de los archivos ISAM y cómo leerlos |
| `03-DATA-DICTIONARY.md` | Diccionario de datos: cada archivo, qué contiene, estructura de campos |
| `04-DOTNET-API.md` | API de las DLLs .NET (limitada pero útil para inicialización) |
| `05-SETUP-AND-FIXES.md` | Instalación, licencias, problemas resueltos |
| `06-MIDDLEWARE-STRATEGY.md` | Estrategia para el middleware Siigo → Web |

## Datos de ejemplo que logramos leer

**Terceros (Z17):**
- "PROVEEDORES"
- "SUPERMERCADOS LA GRAN ESTRELLA"
- Seguros contra incendio, contra terremoto
- Mantenimiento software

**Movimientos (Z49):**
- "CRUCE DE CUENTAS S/N ANEXO"
- "ABONO A FACT. CTA 01"
- "ANTICIPO DE CLIENTE. CTA 01"
- "CANCELA FACT. Y SALDO A FAVOR DEL CLIENTE"

**Facturación electrónica (Z90ES):**
- "Facturación electrónica"
- "Elabora tu factura de venta electrónica"
- "Elabora notas crédito/débito electrónicas"
- "Documento soporte electrónico"

**Comprobantes (Z70):**
- "FAVOR COMUNICARSE CON EL CLIENTE Y ACUERDO PAGO"
- "LLAMAR NUEVAMTE"
