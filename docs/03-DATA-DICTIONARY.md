# Siigo - Diccionario de Datos

Directorio de datos: `C:\DEMOS01\` (configurado en `C:\Siigo\FILEPATH.TXT`)

## Convenciones de Nomenclatura

- **Z** = Archivos de datos principales del sistema
- **C** = Archivos de configuración/catálogos
- **N** = Archivos de nómina
- **YYYY** = Sufijo de año fiscal (ej: Z032016 = movimientos contables del 2016)
- Archivos sin extensión = datos ISAM
- Archivos `.idx` = índices ISAM
- Archivos `.mdb` = bases de datos Access (para informes web SIIWI)

---

## DATOS MAESTROS

### Z17 — Terceros (Clientes, Proveedores, Empleados)
**El archivo más importante para órdenes de compra.**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 1,438 bytes | |
| Total registros | 73 (empresa demo) | |

**Estructura observada del registro:**
```
Pos   Largo  Campo                 Ejemplo
─────────────────────────────────────────────────────
0     1      Tipo clave            G, L, N
1     3      Código empresa        001
4     13     Código del tercero    0000000000020
17    4      Secuencial            01
21    ?      Tipo ID               1=CC, 3=NIT, etc.
?     2      Código tipo doc       13
?     10     Número documento      3005000020
?     8      Fecha creación        20121030
?     40     Nombre/Razón social   "PROVEEDORES"
?     60     Dirección
?     30     Ciudad
?     15     Teléfono
?     15     Teléfono 2
?     1      Estado                (activo/inactivo)
```

**Tipos de clave observados:**
- `G` = Registro general/maestro del tercero (datos principales)
- `L` = Línea/detalle (datos adicionales, configuración contable)
- `N` = Registro por NIT/CC (acceso por documento)

**Datos reales encontrados:**
```
G001...002001 → "PROVEEDORES" (proveedor genérico)
G001...002002 → "SUPERMERCADOS LA GRAN ESTRELLA" (cliente)
L001...003001 → "DOS" (parcial - configuración)
L002...004001 → "RO CONTRA INCENDIO" (seguro)
L002...004003 → "A MANTENIMIENTO SOFTWARE" (gasto)
L002...004005 → "RO CONTRA TERREMOTO" (seguro)
L002...004007 → "ROS GENERALES" (gastos generales)
```

**Nota:** Los nombres aparecen truncados al inicio porque el scanner lee desde el marcador de registro que está unos bytes antes del nombre real. El nombre completo está en la posición correcta dentro del registro.

---

### Z06 — Productos / Inventario
**Clave para el catálogo de productos.**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 4,096 bytes (2,286 en backup) | Registro grande |
| Total registros | ~585 | |

**Campos típicos de un producto en Siigo:**
```
- Código del producto (hasta 20 caracteres)
- Nombre/descripción (hasta 60 caracteres)
- Unidad de medida
- Grupo de inventario
- Cuenta contable de inventario
- Cuenta de costo de ventas
- Cuenta de ingreso
- Precio de venta 1-5
- IVA aplicable
- Estado (activo/inactivo)
- Bodega
- Tipo de producto
```

**Archivos relacionados con productos:**
| Archivo | Contenido |
|---------|-----------|
| `Z06` | Maestro de productos |
| `Z06CP` | Lista de precios por producto |
| `Z06MCCO` | Códigos internos |
| `Z06MCON` | Códigos contables |
| `ZM06` | Movimientos de inventario |
| `Z0620171114` | Backup del maestro de productos (fecha 2017-11-14) |

**Datos encontrados:**
```
"PUBLICIDAD" (producto/servicio)
"EXCELENTE" (clasificación o producto)
"ELEVIDOR LD 32 PULGADAS" (producto)
```

---

### C03 — Plan de Cuentas Contable (PUC)

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 512 bytes | |
| Total registros | 17 (agrupadores de la empresa) | |
| PUC completo | `C:\Siigo\Z03` (191KB) | |

**Estructura de clave:**
```
D = Agrupador/División
F = Cuenta individual

Formato: Tipo(1) + Empresa(3) + CódigoCuenta(13) + Secuencial
```

**Cuentas observadas:**
```
1105 - (Caja)
1355 - (Anticipos y avances)
2365 - (Retención en la fuente)
2408 - (IVA por pagar)
4205 - (Ingresos)
```

**Estructura del PUC colombiano (referencia):**
```
1xxx - Activos
2xxx - Pasivos
3xxx - Patrimonio
4xxx - Ingresos
5xxx - Gastos
6xxx - Costos de ventas
7xxx - Costos de producción
8xxx - Cuentas de orden deudoras
9xxx - Cuentas de orden acreedoras
```

---

### ZDANE — Códigos DANE de Ciudades

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 256 bytes | |
| Total registros | 1,384 | Todos los municipios de Colombia |

Contiene códigos DANE oficiales para todos los municipios de Colombia (usado en facturación electrónica DIAN).

---

### Z003 — Usuarios del Sistema

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 61 bytes | |
| Total registros | 1+ | |
| BLOQUEADO cuando Siigo está abierto | | |

**Estructura:**
```
Pos  Largo  Campo
──────────────────────
0    1      Estado
1    1      Separador "="
2    3      Empresa "001"
5    8      Usuario "ADMON   "
13   40     Nombre "ADMINISTRADOR"
53   8      Padding/datos adicionales
```

**Usuario por defecto:** ADMON / ADMINISTRADOR

**Nota:** Las contraseñas NO están en Z003. Están cifradas en archivos separados.

---

## DATOS TRANSACCIONALES

### Z49 — Movimientos / Transacciones
**El archivo transaccional principal. Contiene TODOS los movimientos: facturas, notas, recibos, etc.**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 2,295 bytes | |
| Total registros | 3,235 | |

**Campos típicos de un movimiento:**
```
- Tipo de comprobante
- Número de documento
- Fecha
- NIT/CC del tercero
- Cuenta contable
- Centro de costo
- Débito/Crédito
- Descripción (hasta ~80 caracteres)
- Referencia
```

**Datos reales encontrados:**
```
"CRUCE DE CUENTAS S/N ANEXO."
"ABONO A FACT. CTA 01."
"ANTICIPO DE CLIENTE. CTA 01."
"CANCELA FACT. Y SALDO A FAVOR DEL CLIENTE. CTA 01."
```

**Archivos por año:**
- `Z492013`, `Z492014`, `Z492015`, `Z492016` — Movimientos archivados por año
- `Z49A2013`, `Z49A2014` — Auxiliar de movimientos por año

---

### Z03YYYY — Movimientos Contables por Año

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 1,152 bytes | |
| Total registros | ~2,851 por año | |
| Archivos | Z032013, Z032014, Z032015, Z032016 | |

Cada registro es un asiento contable con: cuenta, débito, crédito, tercero, centro de costo, fecha, descripción.

---

### Z04YYYY — Detalle de Movimientos por Año

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | 3,520 bytes | |
| Total registros | ~125 por año | |
| Archivos | Z042013, Z042014, Z042015, Z042016 | |

Detalle ampliado de los movimientos contables.

---

### Z70 — Comprobantes / Agenda de Cobro

| Campo | Tipo | Descripción |
|-------|------|-------------|
| Tamaño registro | ~300 bytes | |
| Total registros | 4 | |

**Estructura observada:**
```
NIT(13) + Código(10) + Fecha(8) + Secuencial + Descripción
```

**Datos:**
```
NIT 0000830045678000 → "FAVOR COMUNICARSE CON EL CLIENTE Y ACUERDO PAGO"
NIT 0000830077888000 → "LLAMAR NUEVAMTE"
Fechas: 20140725, 20140623
```

---

## CARTERA (Cuentas por Cobrar y Pagar)

### Z09YYYY — Cartera por Año
| Tamaño registro | 1,152 bytes |
| Registros (2013) | 587 |

### Z09CLYYYY — Clasificación de Cartera
### Z09DLAYYYY — Detalle de Cartera
### Z09H — Histórico de Cartera

---

## CONFIGURACIÓN DEL SISTEMA

### Z90ES — Módulos y Permisos (1,824 registros)

Cada registro describe un módulo o función del sistema:

```
Formato: ModuloNombre(16) + Código(16) + Descripción(80)
```

**Módulos del sistema Siigo:**
| Código | Módulo |
|--------|--------|
| 02 | Contabilidad |
| 03 | Cuentas por Cobrar |
| 04 | Cuentas por Pagar |
| 05 | Inventario |
| 07 | Activos Fijos |
| 11 | Ventas |
| 18 | Mercadeo |
| 24 | Interfases |
| 30 | Tesorería |
| 33 | Manejo de Cartera |
| 39 | Explosión de materiales |
| 51 | Facturación (Electrónica) |

**Funciones de Facturación Electrónica encontradas:**
- Facturación electrónica
- Elaborar factura de venta electrónica
- Elaborar notas crédito electrónicas
- Elaborar notas débito electrónicas
- Documento soporte electrónico
- Certificado digital
- Sincronizar resolución de facturación
- Informe de facturación electrónica y descarga de XML
- Aceptación tácita de la factura
- Validación de identidad

### Z91PRO — Catálogo de Programas (4,943 registros)

Describe cada programa/módulo del sistema:
```
CódigoPrograma(6) + Secuencial(6) + CódigoRepetido(6) + Descripción(40+)
```

**Ejemplos:**
```
Z271A  → "Libro diario"
250C   → "Importar catálogo de empleados"
AUDITO → "EJECUTABLE AUDITOR EN WINDOWS"
C000   → "Instalador de cajas"
C003   → "Registro de control cajero"
C012   → "Parametros documentos"
C013   → "Parametrizacion CONCEPTOS"
C014   → "Parametros inicales"
```

### Z9001ES — Configuración del Sistema (1,056 registros)
| Tamaño registro | 260 bytes |

### C05 — Parámetros de la Empresa
| Tamaño registro | 81 bytes |

### C06 — Configuración de Cajas / Puntos de Venta
| Tamaño registro | ~1,553 bytes |
| Registros | ~48 |

Contiene: nombre de caja ("CAJA UNICENTRO 01"), tipo de impresora (Epson), puerto (COM1), configuración de POS.

### Z001 — Datos de la Empresa
| Tamaño registro | ~3,080 bytes |
| Registros | 1 |

---

## NÓMINA

### N03YYYY — Movimientos de Nómina
| Tamaño registro | 1,152 bytes |
| Registros | ~453 por año |

### N04YYYY — Detalle de Nómina
| Tamaño registro | 3,520 bytes |
| Registros | ~125 por año |

### ZPILA — PILA (Planilla Integrada de Liquidación de Aportes)
Aportes a seguridad social, pensión, ARL, cajas de compensación.

---

## OTROS ARCHIVOS

### Informes
| Archivo | Descripción | Registros |
|---------|-------------|-----------|
| INF | Definiciones de informes | 183 |
| ZCUBOS | Cubos de información | variable |
| SIIWI*.mdb | Bases Access para informes web | N/A |

### Impuestos y Retenciones
| Archivo | Descripción |
|---------|-------------|
| ZIVA | Tarifas de IVA |
| ZRET | Retenciones |
| ZRES | Resoluciones de facturación |

### Otros
| Archivo | Descripción |
|---------|-------------|
| ZPAIS | Tabla de países |
| ZIND | Indicadores |
| ZPRE | Presupuestos |
| ZFLUJ | Flujos |
| ZVEH | Vehículos |
| Z24 | Activos fijos maestro |
| Z12x | Nómina - empleados/conceptos |
| Z120 | Empleados (2,139 registros) |
| Z122 | Conceptos nómina (1,146 registros) |

---

## RESUMEN: TOP 30 ARCHIVOS POR TAMAÑO

```
Archivo              Tamaño         RecSize  Registros  Descripción
──────────────────────────────────────────────────────────────────────
Z49                  7,525,888      2,295    3,235      Movimientos/transacciones
Z032013              3,691,488      1,152    2,852      Mov. contables 2013
Z032014              3,691,488      1,152    2,851      Mov. contables 2014
Z032015              3,691,488      1,152    2,851      Mov. contables 2015
Z032016              3,691,488      1,152    2,851      Mov. contables 2016
Z06                  2,508,552      4,096    ~585       Productos/inventario
Z91PRO               2,328,496      254      4,943      Catálogo programas
Z0620171114          1,415,088      2,286    585        Backup productos
Z90ES                1,134,976      250      1,824      Módulos/permisos
Z092013              851,408        1,152    587        Cartera 2013
Z9001ES              695,296        260      1,056      Configuración
Z12201               688,064        512      1,280      Nómina
Z120                 647,184        256      2,139      Empleados
Z122                 615,312        512      1,146      Conceptos nómina
N032013-N032016      591,664 c/u    1,152    ~453       Nómina mov. por año
N042014-N042016      479,424 c/u    3,520    ~125       Nómina det. por año
Z042013-Z042016      479,424 c/u    3,520    ~125       Detalle mov. por año
Z11I2013             409,352        516      545        (por investigar)
ZDANE                403,696        256      1,384      Ciudades Colombia
INF                  344,080        1,664    183        Informes
Z082014              286,944        2,560    93         (por investigar)
```
