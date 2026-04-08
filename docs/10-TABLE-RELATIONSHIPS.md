# Tablas Activas — Siigo → Finearom Integration

## Contexto

El middleware Siigo lee archivos ISAM de Siigo Pyme y los importa a SQLite. Originalmente se importaban 27 tablas, pero tras una auditoria tabla por tabla (2026-03-20) se redujo a **13 tablas activas** + 1 oculta, eliminando todo lo que era pura contabilidad interna sin valor para la plataforma B2B Finearom.

**Criterio de seleccion**: Solo se mantienen tablas que aporten informacion de negocio util para la plataforma de ordenes de compra, gestion de clientes, productos, facturacion, y formulaciones de aromas/fragancias.

---

## 13 Tablas Activas

### Tablas Maestras (datos base)

| # | Tabla | ISAM | Registros | Descripcion | Valor para Finearom |
|---|-------|------|-----------|-------------|---------------------|
| 1 | **clients** | Z08A | ~1230 | Terceros/Clientes completos | NIT, nombre, DV, tipo persona, rep legal, email, direccion. Enriquecida automaticamente desde Z11N (audit trail) |
| 2 | **products** | Z04 | ~1476 | Catalogo de productos/aromas | Codigo, nombre, nombre corto, referencia, grupo |
| 3 | **codigos_dane** | ZDANE | 1119 | Municipios colombianos | Validar/normalizar ciudades de clientes y sucursales |

### Tablas Transaccionales (movimiento de negocio)

| # | Tabla | ISAM | Registros | Descripcion | Valor para Finearom |
|---|-------|------|-----------|-------------|---------------------|
| 4 | **cartera** | Z09 | ~303 | Cuentas por cobrar/pagar | Saldo pendiente por cliente, fechas de vencimiento |
| 5 | **documentos** | Z11 | ~4248 | Facturas y docs contables | **Valor facturado por cliente** (BCD), NIT real extraido de cuenta_contable. Tipo F = facturas |
| 6 | **condiciones_pago** | Z05 | ~499 | Terminos de pago por cliente | Plazos, montos, fechas de registro |
| 7 | **formulas** | Z06 tipo R | ~2461 | Recetas/formulaciones | Ingredientes por producto con % composicion. Complementa reference_formula_lines de Finearom |

### Tablas de Soporte (contabilidad util)

| # | Tabla | ISAM | Registros | Descripcion | Valor para Finearom |
|---|-------|------|-----------|-------------|---------------------|
| 8 | **plan_cuentas** | Z03 | ~697 | Plan Unico de Cuentas PUC | Nombres de cuentas contables para reportes |
| 9 | **saldos_terceros** | Z25 | ~3958 | Saldos por cliente+cuenta (BCD) | Dashboard financiero: "este cliente debe X" |
| 10 | **libros_auxiliares** | Z07 | ~1041 | Detalle contable por cuenta | Auditoria detallada de movimientos |
| 11 | **historial** | Z18 | ~319 | Historia de documentos | Trazabilidad de cambios |
| 12 | **maestros** | Z06 | ~665 | Centros costo, bodegas, config | Referencia para bodegas y CC |
| 13 | **vendedores_areas** | Z06A | ~6 | Vendedores y areas comerciales | Enriquecer datos de ejecutivos en Finearom |

### Tabla Oculta (solo para enriquecimiento interno)

| Tabla | ISAM | Descripcion |
|-------|------|-------------|
| **audit_trail_terceros** | Z11N | Log de cambios por tercero. No tiene UI ni API propia. Se usa en PostDetect para enriquecer clients con: tipo_doc, rep legal, fecha ultimo cambio, usuario |

---

## 12 Tablas Eliminadas (2026-03-20)

| Tabla | ISAM | Razon de eliminacion |
|-------|------|---------------------|
| movements | Z49 | Indice de comprobantes sin valores. Redundante con documentos (Z11) que SI tiene BCD valor |
| saldos_consolidados | Z28 | Saldos por cuenta SIN NIT — solo para contabilidad, no identifica clientes |
| transacciones_detalle | Z07T | Complemento tecnico de libros_auxiliares, demasiado granular |
| periodos_contables | Z26 | Configuracion de periodos fiscales — irrelevante para B2B |
| clasificacion_cuentas | Z279CP | Mapeo PUC interno — solo para contadores |
| activos_fijos | Z27 | Muebles/equipos de la empresa — nada que ver con aromas |
| activos_fijos_detalle | Z27A | Idem |
| actividades_ica | ZICA | Codigos de impuesto ICA — solo para declaraciones |
| conceptos_pila | ZPILA | Nomina/seguridad social — cero relacion con ventas |
| movimientos_inventario | Z16 | VACIO (0 registros en la empresa) |
| saldos_inventario | Z15 | VACIO (0 registros en la empresa) |
| docs_inventario | Z11I | Audit trail de cambios en productos — tecnico, sin valor de negocio |

> **Nota**: Los datos de estas tablas siguen en SQLite (no se borraron), pero ya no se detectan, sincronizan, ni aparecen en la UI/API/OData.

---

## Relaciones entre Tablas Activas

### Diagrama

```
                    ┌──────────────────┐
                    │   plan_cuentas   │
                    │  (codigo_cuenta) │
                    └────────┬─────────┘
                             │
          ┌──────────────────┼──────────────────┐
          │                  │                  │
          ▼                  ▼                  ▼
   ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐
   │   cartera    │  │  documentos  │  │ saldos_terceros   │
   │ (nit,cuenta) │  │(nit,cuenta,  │  │  (nit, cuenta)    │
   └──────┬───────┘  │ valor, nit_  │  └───────┬───────────┘
          │          │ cliente)     │          │
          │          └──────┬───────┘          │
          │                 │                  │
          ▼                 ▼                  ▼
   ┌──────────────────────────────────────────────────────┐
   │                     clients                          │
   │              (nit) — tabla central                   │
   │  Enriquecida desde: Z08A + Z11N (audit trail)       │
   └──────────┬─────────────────┬─────────────────────────┘
              │                 │
              ▼                 ▼
   ┌──────────────┐     ┌──────────────────┐
   │condiciones   │     │ vendedores_areas │
   │  _pago (nit) │     │     (nit)        │
   └──────────────┘     └──────────────────┘

   ┌──────────────┐
   │   products   │ ◄── codigo
   │   (code)     │
   └──────┬───────┘
          │
          ▼
   ┌──────────────┐
   │   formulas   │  producto + ingrediente → products
   │(cod_prod,    │
   │ cod_ingred)  │
   └──────────────┘

   Independientes: codigos_dane, historial, maestros, libros_auxiliares
```

### Mapa de FK logicas

| Tabla origen | Campo FK | Tabla destino | Campo destino |
|-------------|----------|---------------|---------------|
| cartera | nit_tercero | clients | nit |
| cartera | cuenta_contable | plan_cuentas | codigo_cuenta |
| documentos | nit_tercero | clients | nit |
| documentos | nit_cliente | clients | nit |
| documentos | cuenta_contable | plan_cuentas | codigo_cuenta |
| saldos_terceros | nit_tercero | clients | nit |
| saldos_terceros | cuenta_contable | plan_cuentas | codigo_cuenta |
| condiciones_pago | nit | clients | nit |
| libros_auxiliares | nit_tercero | clients | nit |
| libros_auxiliares | cuenta_contable | plan_cuentas | codigo_cuenta |
| formulas | codigo_producto | products | code |
| formulas | codigo_ingrediente | products | code |
| vendedores_areas | nit | clients | nit |

---

## Vinculacion Siigo ↔ Finearom

### Mapeo de entidades

| Siigo (SQLite) | Finearom (Laravel/MySQL) | Clave de union | Tipo |
|----------------|------------------------|----------------|------|
| clients.nit | siigo_clients.nit → clients.nit | NIT (normalizado) | Sync directo |
| products.code | siigo_products.codigo → products.code | Codigo producto | Sync directo |
| cartera.nit_tercero | siigo_cartera → cartera | NIT | Sync directo |
| documentos.nit_cliente | clients.nit → purchase_orders.client_id | NIT del cliente facturado | **Cruce OC vs Factura** |
| formulas.codigo_producto | reference_formula_lines.product_id | Codigo producto | Complementa formulaciones |
| condiciones_pago.nit | clients.payment_type, credit_term | NIT | Enriquecer terminos de pago |
| vendedores_areas.codigo | executives | Codigo vendedor | Enriquecer ejecutivos |
| codigos_dane.codigo | clients.city (branch_offices) | Codigo DANE | Validar ciudades |

### Flujo de datos actual

```
Siigo ISAM → Go Middleware (SQLite) → API REST → Finearom Laravel
                                                    │
                                                    ├── siigo_clients    (Z08A)
                                                    ├── siigo_products   (Z04)
                                                    ├── siigo_movements  (ya no se envia)
                                                    └── siigo_cartera    (Z09)
```

---

## Posibles Integraciones Futuras

### 1. Cruce Ordenes de Compra vs Facturas
**Problema**: "Hice una OC en Finearom para el cliente X, ¿cuanto se le facturo realmente en Siigo?"

```sql
-- Desde Siigo: total facturado por cliente (tipo F = facturas, D = debito = cargo)
SELECT nit_cliente, SUM(CASE WHEN tipo_mov='D' THEN valor ELSE 0 END) as total_facturado
FROM documentos
WHERE tipo_comprobante='F'
GROUP BY nit_cliente

-- Cruce con Finearom: OC vs facturado
-- purchase_orders.total vs documentos.valor por nit_cliente
```

**Implementacion**: Endpoint en Laravel que reciba NIT + rango de fechas y devuelva el total facturado desde Siigo. Mostrar en la vista de OC: "Facturado en Siigo: $X.XXX.XXX"

### 2. Sincronizar Formulaciones (formulas → reference_formula_lines)
**Problema**: Finearom tiene `reference_formula_lines` y `corazon_formula_lines` para recetas, pero las formulaciones maestras viven en Siigo (Z06 tipo R).

**Implementacion**: Sync de formulas a Finearom:
- `formulas.codigo_producto` → buscar product en Finearom
- `formulas.codigo_ingrediente` → buscar ingrediente (tambien product)
- `formulas.porcentaje` → porcentaje de composicion
- Crear/actualizar reference_formula_lines automaticamente

### 3. Dashboard Financiero por Cliente
**Problema**: "¿Cuanto nos debe este cliente? ¿Cual es su historial de pagos?"

Usando `saldos_terceros` + `cartera`:
- Saldo actual (debito - credito) por NIT
- Facturas pendientes con fecha de vencimiento
- Historial de pagos (condiciones_pago)

Mostrar en Finearom como tab "Financiero" dentro del detalle del cliente.

### 4. Enriquecer Clientes desde Siigo
**Ya parcialmente hecho**: clients tiene NIT, nombre, DV, rep legal, email desde Z08A + Z11N.

**Pendiente**: Sincronizar campos adicionales a Finearom:
- `clients.dv` → validar digito verificacion
- `clients.tipo_persona` → persona natural/juridica
- `clients.direccion`, `clients.email` → actualizar si Finearom no tiene
- `condiciones_pago` → actualizar payment_type/credit_term en Finearom

### 5. Validar Productos Siigo vs Finearom
**Problema**: ¿Hay productos en Finearom que no existen en Siigo? ¿Los precios coinciden?

Endpoint de reconciliacion: comparar products.code (Siigo) vs products.code (Finearom), reportar diferencias.

### 6. Vendedores → Ejecutivos
**Problema**: Finearom tiene `executives` pero Siigo tiene `vendedores_areas` con areas/centros de costo.

Sincronizar vendedores_areas a executives para mantener actualizado el equipo comercial.

### 7. Codigos DANE → Validacion de Ciudades
**Problema**: Los clientes en Finearom tienen ciudad como texto libre. DANE tiene los codigos oficiales.

Usar codigos_dane para autocompletar y validar ciudades en branch_offices y datos de cliente.

---

## Queries Utiles

### Total facturado por cliente (con nombre)
```sql
SELECT c.nombre, d.nit_cliente,
       SUM(CASE WHEN d.tipo_mov='D' THEN d.valor ELSE 0 END) as debitos,
       SUM(CASE WHEN d.tipo_mov='C' THEN d.valor ELSE 0 END) as creditos
FROM documentos d
JOIN clients c ON c.nit = d.nit_cliente
WHERE d.tipo_comprobante = 'F'
GROUP BY d.nit_cliente
ORDER BY debitos DESC
```

### Receta/formula de un producto
```sql
SELECT f.codigo_producto, p1.nombre as producto,
       f.codigo_ingrediente, p2.nombre as ingrediente,
       f.porcentaje
FROM formulas f
LEFT JOIN products p1 ON p1.code = f.codigo_producto
LEFT JOIN products p2 ON p2.code = f.codigo_ingrediente
WHERE f.codigo_producto = 'XXXXX'
ORDER BY f.porcentaje DESC
```

### Saldo actual de un cliente
```sql
SELECT c.nombre, st.cuenta_contable, st.saldo_anterior, st.debito, st.credito,
       (st.saldo_anterior + st.debito - st.credito) as saldo_actual
FROM saldos_terceros st
JOIN clients c ON CAST(CAST(c.nit AS INTEGER) AS TEXT) = CAST(CAST(st.nit_tercero AS INTEGER) AS TEXT)
WHERE c.nit = '860029997'
```

### Condiciones de pago de un cliente
```sql
SELECT cp.tipo, cp.fecha, cp.valor, cp.fecha_registro
FROM condiciones_pago cp
WHERE CAST(CAST(cp.nit AS INTEGER) AS TEXT) = '860029997'
ORDER BY cp.fecha DESC
```
