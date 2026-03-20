# Relaciones entre Tablas - Siigo Sync

## Resumen

El sistema tiene **27 tablas de datos** sincronizadas desde archivos ISAM de Siigo Pyme. Las relaciones son logicas (SQLite no soporta ALTER TABLE ADD FOREIGN KEY), implementadas mediante **indexes** y **views con NIT normalization**.

> **Nota**: Los NIT en Siigo tienen padding diferente segun la tabla (8 vs 13 digitos). Las views usan `CAST(CAST(x AS INTEGER) AS TEXT)` para normalizar y hacer JOIN correctamente.

---

## Tablas Maestras (fuentes de FK)

| Tabla | Clave Primaria | Descripcion | Archivo ISAM |
|-------|---------------|-------------|--------------|
| **clients** | `nit` | Terceros / Clientes | Z17 |
| **products** | `code` | Productos / Inventario | Z04 |
| **plan_cuentas** | `codigo_cuenta` | Plan Unico de Cuentas (PUC) | Z03 |
| **terceros_ampliados** | `nit` | Datos extendidos de terceros (email, direccion) | Z08A |
| **codigos_dane** | `codigo` | Municipios colombianos (referencia) | ZDANE |
| **actividades_ica** | `codigo` | Actividades ICA con tarifas (referencia) | ZICA |
| **maestros** | `record_key` | Config: centros costo, bodegas, sucursales | Z06 |

---

## Mapa Completo de Relaciones

### clients.nit ← (11 tablas referencian clientes)

```
clients.nit
  ├── movements.nit_tercero          (movimientos contables del tercero)
  ├── cartera.nit_tercero            (cuentas por cobrar del tercero)
  ├── documentos.nit_tercero         (facturas/docs del tercero)
  ├── saldos_terceros.nit_tercero    (saldos por cuenta del tercero)
  ├── transacciones_detalle.nit_tercero  (detalle transacciones)
  ├── libros_auxiliares.nit_tercero  (libros auxiliares del tercero)
  ├── condiciones_pago.nit           (condiciones de pago)
  ├── activos_fijos.nit_responsable  (activos fijos asignados)
  ├── activos_fijos_detalle.nit_responsable (detalle activos)
  ├── audit_trail_terceros.nit_tercero (cambios en datos del tercero)
  └── vendedores_areas.nit           (vendedores/areas del tercero)
```

### terceros_ampliados.nit ↔ clients.nit

```
terceros_ampliados.nit ↔ clients.nit
  (mismo tercero, datos extendidos: email, direccion, rep. legal)
```

### plan_cuentas.codigo_cuenta ← (7 tablas referencian cuentas)

```
plan_cuentas.codigo_cuenta
  ├── movements.cuenta_contable          (cuenta del movimiento)
  ├── cartera.cuenta_contable            (cuenta de cartera)
  ├── documentos.cuenta_contable         (cuenta del documento)
  ├── saldos_terceros.cuenta_contable    (saldo por cuenta)
  ├── saldos_consolidados.cuenta_contable (saldo consolidado)
  ├── transacciones_detalle.cuenta_contable (cuenta transaccion)
  └── libros_auxiliares.cuenta_contable  (cuenta libro auxiliar)
```

### products.code ← (5 tablas referencian productos)

```
products.code
  ├── formulas.codigo_producto           (producto principal de formula)
  ├── formulas.codigo_ingrediente        (ingrediente de formula)
  ├── documentos.producto_ref            (producto en documento)
  ├── movimientos_inventario.codigo_producto (mov. inventario)
  ├── saldos_inventario.codigo_producto  (saldo inventario)
  └── docs_inventario.codigo_producto    (doc inventario)
```

### clasificacion_cuentas → plan_cuentas

```
clasificacion_cuentas.codigo_cuenta → plan_cuentas.codigo_cuenta
  (clasificacion/grupo de la cuenta contable)
```

---

## Diagrama de Relaciones

```
                    ┌──────────────────┐
                    │   plan_cuentas   │
                    │  (codigo_cuenta) │
                    └────────┬─────────┘
                             │
          ┌──────────────────┼──────────────────────────────────┐
          │                  │                                  │
          ▼                  ▼                                  ▼
   ┌─────────────┐  ┌──────────────┐  ┌───────────────────┐  ┌──────────────────┐
   │  movements  │  │   cartera    │  │ saldos_terceros   │  │saldos_consolidados│
   │ (nit,cuenta)│  │ (nit,cuenta) │  │  (nit, cuenta)    │  │    (cuenta)      │
   └──────┬──────┘  └──────┬───────┘  └───────┬───────────┘  └──────────────────┘
          │                │                   │
          ▼                ▼                   ▼
   ┌──────────────────────────────────────────────────────┐
   │                     clients                          │
   │                     (nit)                            │
   └──────────────────────┬───────────────────────────────┘
          │               │               │
          ▼               ▼               ▼
   ┌─────────────┐ ┌────────────┐ ┌──────────────────┐
   │  documentos │ │ condiciones│ │ audit_trail       │
   │(nit,cuenta, │ │   _pago    │ │   _terceros       │
   │ producto)   │ │  (nit)     │ │  (nit_tercero)    │
   └──────┬──────┘ └────────────┘ └──────────────────┘
          │
          ▼
   ┌──────────────┐    ┌──────────────────┐
   │   products   │◄───│ terceros_ampliados│
   │   (code)     │    │  (nit ↔ clients) │
   └──────┬───────┘    └──────────────────┘
          │
   ┌──────┼─────────────────────┐
   │      │                     │
   ▼      ▼                     ▼
┌────────┐ ┌──────────────┐ ┌───────────────────┐
│formulas│ │movimientos   │ │ saldos_inventario │
│(prod,  │ │ _inventario  │ │  (producto)       │
│ ingred)│ │ (producto)   │ └───────────────────┘
└────────┘ └──────────────┘

   ┌────────────────┐    ┌────────────────────┐
   │ activos_fijos  │    │activos_fijos_detalle│
   │(nit_responsable│    │(nit_responsable)    │
   └────────────────┘    └────────────────────┘
          │                        │
          └───────►clients◄────────┘

   ┌───────────────────────┐  ┌──────────────────────┐
   │ transacciones_detalle │  │  libros_auxiliares    │
   │ (nit, cuenta)         │  │  (nit, cuenta)        │
   └───────────────────────┘  └──────────────────────┘
          │    │                      │    │
          ▼    ▼                      ▼    ▼
       clients plan_cuentas       clients plan_cuentas
```

---

## Tablas Independientes (sin FK)

| Tabla | Descripcion | Archivo ISAM |
|-------|-------------|--------------|
| **codigos_dane** | Municipios colombianos (tabla referencia) | ZDANE |
| **actividades_ica** | Actividades ICA con tarifas (tabla referencia) | ZICA |
| **conceptos_pila** | Conceptos PILA seguridad social (tabla referencia) | ZPILA |
| **periodos_contables** | Periodos contables con saldos BCD | Z26 |
| **historial** | Historial de documentos transaccionales | Z18 |
| **maestros** | Maestros de config (sucursales, bodegas, etc.) | Z06 |

---

## Views SQL (pre-configuradas)

Las views resuelven automaticamente las relaciones con NIT normalization:

| View | Tablas que une | Campos agregados |
|------|---------------|-----------------|
| `v_cartera_detalle` | cartera + clients + plan_cuentas | nombre_tercero, nombre_cuenta |
| `v_documentos_detalle` | documentos + clients + plan_cuentas + products | nombre_tercero, nombre_cuenta, nombre_producto |
| `v_saldos_terceros_detalle` | saldos_terceros + clients + plan_cuentas | nombre_tercero, nombre_cuenta |
| `v_libros_auxiliares_detalle` | libros_auxiliares + clients + plan_cuentas | nombre_tercero, nombre_cuenta |
| `v_formulas_detalle` | formulas + products (x2) | nombre_producto, nombre_ingrediente |
| `v_movements_detalle` | movements + clients + plan_cuentas | nombre_tercero, nombre_cuenta |
| `v_saldos_consolidados_detalle` | saldos_consolidados + plan_cuentas | nombre_cuenta |

### Ejemplo de uso

```sql
-- Cartera con nombre del tercero y cuenta
SELECT * FROM v_cartera_detalle WHERE nombre_tercero LIKE '%FINEAROM%';

-- Formulas con nombre de producto e ingrediente
SELECT * FROM v_formulas_detalle WHERE nombre_producto LIKE '%ESENCIA%';

-- Movimientos con nombre del cliente
SELECT * FROM v_movements_detalle WHERE fecha >= '20260101';
```

---

## Indexes de Relacion

Cada FK logica tiene un index para optimizar JOINs:

| Index | Tabla | Columna |
|-------|-------|---------|
| idx_movements_nit_fk | movements | nit_tercero |
| idx_movements_cuenta | movements | cuenta_contable |
| idx_cartera_cuenta | cartera | cuenta_contable |
| idx_activos_fijos_nit | activos_fijos | nit_responsable |
| idx_documentos_producto | documentos | producto_ref |
| idx_transacciones_detalle_cuenta | transacciones_detalle | cuenta_contable |
| idx_transacciones_detalle_nit | transacciones_detalle | nit_tercero |
| idx_libros_auxiliares_cuenta | libros_auxiliares | cuenta_contable |
| idx_libros_auxiliares_nit | libros_auxiliares | nit_tercero |
| idx_condiciones_pago_nit | condiciones_pago | nit |
| idx_activos_fijos_detalle_nit | activos_fijos_detalle | nit_responsable |
| idx_audit_trail_terceros_nit | audit_trail_terceros | nit_tercero |
| idx_vendedores_areas_nit | vendedores_areas | nit |
| idx_clasificacion_cuentas_grupo | clasificacion_cuentas | codigo_grupo |
| idx_saldos_consolidados_cuenta | saldos_consolidados | cuenta_contable |
| idx_formulas_producto | formulas | codigo_producto |
| idx_formulas_ingrediente | formulas | codigo_ingrediente |
| idx_docs_inventario_producto | docs_inventario | codigo_producto |
| idx_movimientos_inventario_producto | movimientos_inventario | codigo_producto |
| idx_saldos_inventario_producto | saldos_inventario | codigo_producto |

---

## OData NavigationProperty

Las relaciones estan expuestas en el `$metadata` OData para que Power BI las auto-detecte:

| Tabla origen | Campo FK | Tabla destino | NavigationProperty |
|-------------|----------|---------------|-------------------|
| movements | nit_tercero | clients | Client |
| movements | cuenta_contable | plan_cuentas | CuentaContable |
| cartera | nit_tercero | clients | Client |
| cartera | cuenta_contable | plan_cuentas | CuentaContable |
| saldos_terceros | nit_tercero | clients | Client |
| saldos_terceros | cuenta_contable | plan_cuentas | CuentaContable |
| saldos_consolidados | cuenta_contable | plan_cuentas | CuentaContable |
| documentos | nit_tercero | clients | Client |
| documentos | cuenta_contable | plan_cuentas | CuentaContable |
| terceros_ampliados | nit | clients | Client |
| transacciones_detalle | nit_tercero | clients | Client |
| transacciones_detalle | cuenta_contable | plan_cuentas | CuentaContable |
| libros_auxiliares | nit_tercero | clients | Client |
| libros_auxiliares | cuenta_contable | plan_cuentas | CuentaContable |
| condiciones_pago | nit | clients | Client |
| activos_fijos | nit_responsable | clients | Client |
| activos_fijos_detalle | nit_responsable | clients | Client |
| audit_trail_terceros | nit_tercero | clients | Client |
| clasificacion_cuentas | codigo_cuenta | plan_cuentas | CuentaContable |
| movimientos_inventario | codigo_producto | products | Producto |
| saldos_inventario | codigo_producto | products | Producto |
| formulas | codigo_producto | products | Producto |
| formulas | codigo_ingrediente | products | Ingrediente |
| docs_inventario | codigo_producto | products | Producto |
| vendedores_areas | nit | clients | Client |

---

## Resumen de Tablas (27 total)

| # | Tabla | ISAM | Registros | FK a | Tiene view |
|---|-------|------|-----------|------|------------|
| 1 | clients | Z17 | 73 | - (maestra) | - |
| 2 | products | Z04 | ~126 | - (maestra) | - |
| 3 | movements | Z49 | 3322 | clients, plan_cuentas | v_movements_detalle |
| 4 | cartera | Z09 | var | clients, plan_cuentas | v_cartera_detalle |
| 5 | plan_cuentas | Z03 | 2836 | - (maestra) | - |
| 6 | activos_fijos | Z27 | 18 | clients | - |
| 7 | saldos_terceros | Z25 | 169 | clients, plan_cuentas | v_saldos_terceros_detalle |
| 8 | saldos_consolidados | Z28 | 116 | plan_cuentas | v_saldos_consolidados_detalle |
| 9 | documentos | Z11 | 38 | clients, plan_cuentas, products | v_documentos_detalle |
| 10 | terceros_ampliados | Z08A | 87 | clients | - |
| 11 | movimientos_inventario | Z16 | 27 | products | - |
| 12 | saldos_inventario | Z15 | 12 | products | - |
| 13 | activos_fijos_detalle | Z27A | 18 | clients | - |
| 14 | audit_trail_terceros | Z11N | 152 | clients | - |
| 15 | transacciones_detalle | Z07T | 116 | clients, plan_cuentas | - |
| 16 | periodos_contables | Z26 | 34 | - (independiente) | - |
| 17 | condiciones_pago | Z05 | 22 | clients | - |
| 18 | libros_auxiliares | Z07 | 64 | clients, plan_cuentas | v_libros_auxiliares_detalle |
| 19 | codigos_dane | ZDANE | 1119 | - (referencia) | - |
| 20 | actividades_ica | ZICA | 431 | - (referencia) | - |
| 21 | conceptos_pila | ZPILA | 48 | - (referencia) | - |
| 22 | clasificacion_cuentas | Z279CP | 1 | plan_cuentas | - |
| 23 | historial | Z18 | 12-106 | - (independiente) | - |
| 24 | maestros | Z06 | ~585 | - (maestra/config) | - |
| 25 | formulas | Z06 | var | products (x2) | v_formulas_detalle |
| 26 | docs_inventario | Z11I | var | products | - |
| 27 | vendedores_areas | Z06A | var | clients | - |
