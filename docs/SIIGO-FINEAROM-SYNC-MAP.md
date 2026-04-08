# Siigo → Finearom: Mapa de Datos Utiles para Ordenes de Compra

## Resumen

Finearom es una plataforma B2B de ordenes de compra para aromas/fragancias. Siigo Pyme es el sistema contable COBOL donde se factura. Este documento mapea que datos de Siigo son utiles para Finearom y como aprovecharlos.

---

## 1. CLIENTS (Z08A) → siigo_clients → clients

**Registros Siigo**: 5,113 | **Finearom**: clients

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `nit` | `clients.nit` | Clave de cruce entre ambos sistemas |
| `nombre` | `clients.client_name` | Nombre del cliente |
| `tipo_persona` | `clients.taxpayer_type` | Tipo de contribuyente |
| `direccion` | `clients.address` | Direccion de facturacion |
| `email` | `clients.email` | Email de contacto |
| `dv` | (nuevo) | Digito verificacion del NIT |
| `tipo_doc` | (nuevo) | Tipo documento identidad |

### Cuando detectar cambios
- **Cliente nuevo**: Crear en Finearom como prospecto o cliente activo
- **Email/direccion cambia**: Actualizar datos de contacto y sucursales (branch_offices)
- **Nombre cambia**: Actualizar client_name y business_name

### Valor para dashboards
- Total clientes activos en Siigo vs registrados en Finearom
- Clientes en Siigo que no tienen OC en Finearom (oportunidad comercial)

---

## 2. PRODUCTS (Z04) → siigo_products → products

**Registros Siigo**: 12,299 | **Finearom**: products

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `code` | `products.code` | Codigo unico del producto |
| `nombre` | `products.product_name` | Nombre completo |
| `codigo_plataforma` | `products.code` (match) | Codigo corto extraido del nombre (ej: "TORONJA PLUS - 589453" → 589453) |
| `grupo` | `products.categories` | Clasificacion (fragancias, materias primas, quimicos) |
| `empresa` | - | Empresa de origen |

### Cuando detectar cambios
- **Producto nuevo**: Crear en catalogo Finearom con precio pendiente
- **Nombre cambia**: Actualizar product_name
- **Grupo cambia**: Actualizar categories

### Valor para dashboards
- Catalogo completo sincronizado
- Productos en Siigo sin precio asignado en Finearom

---

## 3. DOCUMENTOS (Z11) → siigo_movements → purchase_orders

**Registros Siigo**: 9,891 | **Finearom**: purchase_orders + purchase_order_product

### ESTA ES LA TABLA MAS IMPORTANTE PARA ORDENES DE COMPRA

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `nit_cliente` | `purchase_orders.client_id` (via NIT) | Identificar el cliente de la factura |
| `valor` | Comparar vs `purchase_order_product.price * quantity` | **Detectar diferencias de precio** (descuentos, ajustes) |
| `descripcion` | Contiene nombre producto + codigo_plataforma | Identificar que producto se facturo |
| `tipo_comprobante` | - | F=Factura, O=Orden, S=Salida, L=Libro |
| `fecha` | Comparar vs `purchase_orders.dispatch_date` | Fecha real de facturacion |
| `referencia` | `purchase_orders.invoice_number` | Numero de comprobante Siigo |
| `codigo_comp` | - | Codigo del comprobante |

### Tipos de comprobante relevantes

| Tipo | Que es | Registros | Relevancia |
|---|---|---|---|
| **F** | Factura de venta | 34 | ALTA - factura real con valor |
| **O** | Orden/Comprobante | 213 | ALTA - orden de compra interna |
| **S** | Salida | 151 | MEDIA - despacho de mercancia |
| **L** | Libro contable | 7,450 | MEDIA - registro contable con ref a OC |

### Cruce clave: OC Finearom vs Factura Siigo

```
Finearom: purchase_order (client_id=SIMONIZ, product=TORONJA PLUS, qty=100, price=$13.70)
Siigo:    documento (nit_cliente=800203984, descripcion="TORONJA PLUS - 589453", valor=$1,233.00)

→ Match por: NIT + codigo_plataforma en descripcion
→ Dashboard: "OC #1234 - Valor esperado: $1,370 - Facturado: $1,233 - Descuento: $137 (10%)"
```

### Cuando detectar cambios
- **Factura nueva (tipo F)**: Buscar OC abierta del mismo NIT en Finearom, actualizar estado a "facturada", guardar invoice_number
- **Orden nueva (tipo O)**: Registrar como nueva orden interna
- **Valor diferente**: Alertar diferencia vs OC original para dashboard de descuentos

---

## 4. CARTERA (Z09) → siigo_cartera → cartera

**Registros Siigo**: 25,746 | **Finearom**: cartera

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `nit_tercero` | Match con `clients.nit` | Identificar cliente |
| `codigo_producto` | Match con `products.code` | Que producto compro |
| `codigo_plataforma` | Match con codigo corto | Codigo plataforma del producto |
| `tipo_mov` | D=Debe, C=Pago | Estado de pago |
| `fecha` | - | Fecha del movimiento |
| `descripcion` | Nombre del producto | Detalle |

### Cuando detectar cambios
- **Nuevo debito (D)**: Cliente tiene deuda nueva → alerta de cartera pendiente
- **Nuevo credito (C)**: Cliente pago → actualizar estado de pago en OC
- **Producto nuevo para un NIT**: El cliente empezo a comprar un producto nuevo

### Valor para dashboards
- Cartera por cliente: total debe vs total pagado
- Productos comprados por cliente (historial real de compras)
- Clientes morosos (debitos sin credito correspondiente)
- Comparar: "Este cliente compra estos productos en Siigo pero no tiene OC en Finearom"

---

## 5. CONDICIONES DE PAGO (Z05) → clients

**Registros Siigo**: 851 | **Finearom**: clients.payment_type, credit_term

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `nit` | `clients.nit` | Identificar cliente |
| `valor` | `clients.credit_term` | Dias de credito (30, 60, 90) |
| `fecha` | - | Desde cuando aplica |

### Cuando detectar cambios
- **Condicion cambia**: Actualizar credit_term del cliente en Finearom
- **Nuevo NIT con condicion**: Cliente nuevo con terminos de pago definidos

### Valor para dashboards
- Clientes con credito > 60 dias (riesgo)
- Cambios en condiciones de pago (alerta: "A este cliente le ampliaron el credito")

---

## 6. FORMULAS (Z06 tipo R) → reference_formula_lines

**Registros Siigo**: 24,842 | **Finearom**: finearom_references + reference_formula_lines

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `codigo_producto` | `finearom_references.codigo` | Producto al que pertenece la formula |
| `codigo_ingrediente` | `raw_materials.codigo` | Ingrediente/materia prima |
| `porcentaje` | `reference_formula_lines.porcentaje` | % del ingrediente en la formula |

### Cuando detectar cambios
- **Formula nueva**: Crear referencia + lineas en Finearom
- **Porcentaje cambia**: Actualizar formula (puede afectar costo)
- **Ingrediente nuevo en formula**: Agregar linea

### Valor para dashboards
- Comparar formulas Siigo vs formulas Finearom (detectar discrepancias)
- Costo estimado por formula basado en raw_materials.costo_unitario
- Productos con mas ingredientes (complejidad)

---

## 7. NOTAS DE DOCUMENTOS (Z49) → purchase_orders

**Registros Siigo**: 2,515 | **Finearom**: purchase_orders (campos adicionales)

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `orden_compra` | `purchase_orders.order_consecutive` | **Numero de OC** - cruce directo! |
| `lote` | (nuevo campo o partials) | Numero de lote de produccion |
| `fecha_despacho` | `purchase_orders.dispatch_date` | Fecha real de despacho |
| `empaque` | (nuevo campo) | Tipo de empaque usado |
| `num_documento` | Ref a documento Z11 | Vincula nota con factura |

### CRUCE DIRECTO CON ORDENES DE COMPRA

Esta tabla es clave porque tiene el campo `orden_compra` que es el numero de OC que puede coincidir con `purchase_orders.order_consecutive` en Finearom.

### Cuando detectar cambios
- **Nota con orden_compra**: Vincular directamente con OC en Finearom
- **Fecha despacho nueva**: Actualizar dispatch_date de la OC
- **Lote asignado**: Registrar trazabilidad del despacho
- **Empaque definido**: Registrar en observaciones de la OC

### Valor para dashboards
- OC con despacho registrado vs OC pendientes
- Tiempo promedio entre OC y despacho
- Trazabilidad: OC → factura → lote → empaque

---

## 8. FACTURAS ELECTRONICAS (Z09ELE) → purchase_orders

**Registros Siigo**: 65 | **Finearom**: purchase_orders

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `nit_tercero` | Match con `clients.nit` | Cliente |
| `valor` | Comparar vs OC | Valor facturado electronicamente |
| `descripcion` | Producto facturado | Detalle |
| `vendedor` | `clients.executive` | Ejecutivo asignado |
| `fecha` | `purchase_orders.dispatch_date` | Fecha factura electronica |

### Cuando detectar cambios
- **Factura electronica nueva**: Actualizar OC con invoice_number y valor real
- **Vendedor asignado**: Actualizar ejecutivo del cliente

### Valor para dashboards
- Facturacion electronica vs facturacion normal
- Seguimiento de vendedores (quien factura que)

---

## 9. DETALLE MOVIMIENTOS (Z17) → purchase_order_product

**Registros Siigo**: 2,247 | **Finearom**: purchase_order_product (lineas de OC)

### Campos que sirven

| Campo Siigo | Campo Finearom | Para que |
|---|---|---|
| `codigo_producto` | `products.code` | Producto en el movimiento |
| `nombre` | Tiene codigo_plataforma en el nombre | Nombre + codigo |
| `tipo_comprobante` | Tipo de movimiento | Entrada/salida |
| `num_comprobante` | Ref a documento | Vincula con factura |
| `bodega` | (nuevo) | Bodega de origen |
| `fecha` | - | Fecha del movimiento |
| `valor` | `purchase_order_product.price` | Valor del movimiento |

### Cuando detectar cambios
- **Movimiento nuevo**: Registrar despacho real de producto
- **Valor del movimiento**: Comparar con precio en OC

### Valor para dashboards
- Productos mas movidos (top ventas reales)
- Movimientos por bodega
- Cruce: valor movimiento Siigo vs valor linea OC Finearom

---

## 10-12. TABLAS COMPLEMENTARIAS

### vendedores_areas (0 registros actualmente)
- **Mapea a**: `clients.executive` / tabla executives
- **Cuando tenga datos**: Sincronizar vendedores como ejecutivos en Finearom

### codigos_dane (0 registros actualmente)
- **Mapea a**: `branch_offices.delivery_city`
- **Cuando tenga datos**: Validar ciudades de sucursales con codigos oficiales DANE

### audit_trail_terceros (0 registros actualmente)
- **Mapea a**: Enriquece `clients` con DV, rep legal, etc.
- **Cuando tenga datos**: Completar datos fiscales de clientes

---

## Resumen de Cruces Clave

```
┌─────────────────────────────────────────────────────────────────┐
│                    FLUJO DE DATOS SIIGO → FINEAROM              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  SIIGO (ISAM)              →  FINEAROM (Laravel)                │
│                                                                  │
│  clients.nit               →  clients.nit (match)               │
│  products.code             →  products.code (match)             │
│  products.codigo_plataforma →  products.code (alt match)        │
│                                                                  │
│  documentos (tipo F/O)     →  purchase_orders                   │
│    .nit_cliente             →    .client_id (via NIT)           │
│    .valor                   →    comparar vs OC.total           │
│    .descripcion             →    identificar producto           │
│    .referencia              →    .invoice_number                │
│    .fecha                   →    .dispatch_date                 │
│                                                                  │
│  notas_documentos          →  purchase_orders                   │
│    .orden_compra            →    .order_consecutive (DIRECTO!)  │
│    .fecha_despacho          →    .dispatch_date                 │
│    .lote                    →    partials.tracking_number       │
│                                                                  │
│  cartera                   →  cartera / dashboards              │
│    .nit_tercero + producto  →    historial compras x cliente    │
│    .tipo_mov D/C            →    estado pago                    │
│                                                                  │
│  condiciones_pago          →  clients                           │
│    .valor (dias)            →    .credit_term                   │
│                                                                  │
│  formulas                  →  reference_formula_lines           │
│    .codigo_producto         →    finearom_references.codigo     │
│    .codigo_ingrediente      →    raw_materials.codigo           │
│    .porcentaje              →    .porcentaje                    │
│                                                                  │
│  facturas_electronicas     →  purchase_orders                   │
│    .valor                   →    valor facturado real           │
│    .vendedor                →    clients.executive              │
│                                                                  │
│  detalle_movimientos       →  purchase_order_product            │
│    .codigo_producto         →    .product_id                    │
│    .valor                   →    comparar vs linea OC           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Dashboards Posibles

### 1. OC vs Facturacion Real
- Valor OC en Finearom vs valor facturado en Siigo
- Descuentos aplicados (diferencia)
- OC sin facturar (pendientes en Siigo)

### 2. Cartera por Cliente
- Total debe / total pagado por NIT
- Dias de mora promedio
- Clientes con cartera vencida

### 3. Productos por Cliente
- Historial real de compras (de cartera Z09)
- Productos nuevos que empezo a comprar
- Top productos por cliente

### 4. Trazabilidad de OC
- OC → factura → lote → despacho (datos de notas_documentos)
- Tiempo promedio OC→despacho
- OC con lote asignado vs pendientes

### 5. Formulas / Costos
- Discrepancias formula Siigo vs Finearom
- Costo estimado basado en ingredientes
- Productos con formula incompleta

### 6. Ejecutivos / Vendedores
- Facturacion por vendedor (de facturas_electronicas)
- Clientes por ejecutivo
