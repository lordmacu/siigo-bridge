# Mapeo de Sincronización: Siigo ↔ Finearom

## Plataforma Web: Finearom

Finearom es una plataforma B2B de gestión de órdenes de compra para una empresa de aromas/fragancias. Stack: Laravel (backend) + API REST.

Ubicación: `C:\laragon\www\finearom\backend`

### Entidades principales de Finearom

| Entidad | Tabla | Campos clave |
|---------|-------|-------------|
| **Clientes** | `clients` | nit, client_name, business_name, email, phone, address, city, payment_type, payment_method, iva, retefuente, reteiva |
| **Productos** | `products` | code, product_name, price, client_id, categories |
| **Órdenes de Compra** | `purchase_orders` | client_id, order_consecutive, order_creation_date, required_delivery_date, status, invoice_number |
| **Productos x Orden** | `purchase_order_product` | product_id, purchase_order_id, quantity, price, delivery_date |
| **Despachos (Parciales)** | `partials` | product_id, order_id, quantity, dispatch_date, invoice_number, tracking_number, transporter |
| **Sucursales** | `branch_offices` | client_id, name, nit, delivery_address, delivery_city |
| **Cartera** | `cartera` | nit, factura, fecha_vencimiento, dias_mora, valor |
| **Recaudos** | `recaudos` | nit, numero_factura, valor_cancelado, fecha_recaudo |
| **Historial Precios** | `product_price_history` | product_id, price, effective_date |
| **Categorías** | `product_categories` | name (Body Care, Home Care, Air Care, Fine Fragrance) |

---

## Mapeo de Sincronización

### 1. CLIENTES (Prioridad: ALTA)

**Dirección: Siigo → Finearom** (sincronizar clientes desde contabilidad)

| Finearom (`clients`) | Siigo (Z17) | Notas |
|----------------------|-------------|-------|
| `nit` | Número de documento (pos ~22, 10 chars) | Clave de cruce principal |
| `client_name` | Nombre/Razón social (pos ~40, 40 chars) | |
| `business_name` | Nombre (mismo campo) | |
| `address` | Dirección (dentro del registro) | |
| `city` | Ciudad | |
| `phone` | Teléfono | |
| `taxpayer_type` | Tipo contribuyente | |
| `iva` | Tarifa IVA | Desde config contable |
| `retefuente` | Retención en la fuente | Desde config contable |
| `reteiva` | Retención de IVA | Desde config contable |

**Archivo Siigo**: `Z17` (1,438 bytes/registro, 73 registros en demo)
**Clave de cruce**: NIT del tercero

**Dirección: Finearom → Siigo** (no recomendado por ahora — riesgo de corrupción ISAM)

---

### 2. PRODUCTOS (Prioridad: ALTA)

**Dirección: Siigo → Finearom** (catálogo maestro de productos)

| Finearom (`products`) | Siigo (Z06) | Notas |
|-----------------------|-------------|-------|
| `code` | Código producto (primeros chars del registro) | |
| `product_name` | Nombre/descripción (~60 chars) | |
| `price` | Precio de venta | Desde Z06CP o campo dentro de Z06 |
| `categories` | Grupo de inventario | Mapear a las 4 categorías de Finearom |

**Archivo Siigo**: `Z06` (4,096 bytes/registro, ~585 productos)
**Backup legible**: `Z0620171114` (2,286 bytes/registro, 585 registros)
**Archivos relacionados**: `Z06CP` (precios), `Z06MCCO`/`Z06MCON` (códigos)

---

### 3. ÓRDENES DE COMPRA (Prioridad: ALTA)

**Dirección: Finearom → Siigo** (las órdenes se crean en la web y deben reflejarse en contabilidad)

Siigo no tiene una tabla directa de "órdenes de compra web" pero los movimientos se registran en:

| Finearom | Siigo | Archivo |
|----------|-------|---------|
| `purchase_orders` → factura | Z49 (movimientos) | Al facturar la orden |
| Productos de la orden | Z49 detalle + Z04YYYY | Líneas del movimiento |
| Totales, IVA, retenciones | Z03YYYY (contable) | Asientos contables |

**Archivos Siigo relevantes**:
- `Z49` — Movimientos/transacciones (2,295 bytes/reg, 3,235 registros)
- `Z03YYYY` — Movimientos contables por año (1,152 bytes/reg)
- `Z04YYYY` — Detalle de movimientos (3,520 bytes/reg)
- `Z90PO` — Pedidos/órdenes en Siigo
- `BOPED.gnt` — Business Object de Pedidos (programa COBOL)

**Nota**: Para crear movimientos en Siigo desde la web, hay dos opciones:
1. **Interfaz de Siigo manual**: El usuario registra la factura manualmente en Siigo
2. **Archivo plano de importación**: Siigo tiene módulo de interfases (módulo 24) que puede importar movimientos desde archivos planos

---

### 4. DESPACHOS (Prioridad: MEDIA)

**Dirección: Finearom → Siigo** (los despachos se registran en la web)

| Finearom (`partials`) | Siigo | Notas |
|----------------------|-------|-------|
| dispatch_date | Fecha movimiento | |
| quantity | Cantidad despachada | |
| invoice_number | Número factura | Referencia cruzada |
| tracking_number | N/A | No existe en Siigo |
| transporter | N/A | No existe en Siigo |

**Archivos Siigo**:
- `ENVIA/` — Directorio de despachos enviados (A06.DIS, 2.1MB)
- `RECIBE/` — Directorio de recepciones (A06.DIS, 19MB)
- `BOEnv.gnt`, `EnviaCobol.gnt`, `ZUENV.gnt` — Programas de envío

---

### 5. CARTERA (Prioridad: MEDIA)

**Dirección: Siigo → Finearom** (la cartera viene de contabilidad)

| Finearom (`cartera`) | Siigo | Archivo |
|---------------------|-------|---------|
| `nit` | NIT del tercero | Z09YYYY |
| Factura | Número de documento | Z09YYYY |
| `fecha_vencimiento` | Fecha vencimiento | Z09YYYY |
| `dias_mora` | Calculado | |
| Valor factura | Saldo | Z09YYYY |

**Archivos Siigo**: `Z09YYYY` (cartera por año, 1,152 bytes/reg, ~587 registros)

**Nota**: Finearom YA importa cartera desde Excel (`POST /cartera/import`). Se podría automatizar generando el Excel desde los archivos ISAM de Siigo.

---

### 6. RECAUDOS (Prioridad: MEDIA)

**Dirección: Siigo → Finearom** (los pagos se registran en contabilidad)

| Finearom (`recaudos`) | Siigo | Archivo |
|----------------------|-------|---------|
| `fecha_recaudo` | Fecha del recibo | Z49 (tipo recibo de caja) |
| `numero_recibo` | Número comprobante | Z49 |
| `numero_factura` | Factura que paga | Z49 referencia |
| `nit` / `cliente` | Tercero | Z17 |
| `valor_cancelado` | Valor | Z49 |

**Nota**: Finearom YA importa recaudos desde Excel (`POST /recaudo/import`).

---

## Estrategia de Sincronización Recomendada

### Fase 1: Lectura (Siigo → Finearom)
Lo que podemos hacer YA con el lector ISAM:

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│   Siigo     │  ISAM   │  Middleware   │  HTTP   │  Finearom    │
│   (COBOL)   │ ──────→ │  (Worker     │ ──────→ │  (Laravel    │
│             │  read   │   Service)   │  POST   │   API)       │
└─────────────┘         └──────────────┘         └──────────────┘
```

| Dato | Archivo Siigo | Endpoint Finearom | Acción |
|------|--------------|-------------------|--------|
| Clientes nuevos/actualizados | Z17 | `POST /api/clients` | Crear/actualizar por NIT |
| Productos nuevos/actualizados | Z06 | `POST /api/products` | Crear/actualizar por código |
| Cartera actualizada | Z09YYYY | `POST /api/cartera/import` | Importar como Excel/JSON |
| Recaudos nuevos | Z49 (filtrado) | `POST /api/recaudo/import` | Importar cobros |

### Fase 2: Escritura (Finearom → Siigo)
Más complejo, opciones:

1. **Archivo plano de importación** (Módulo 24 de Siigo - Interfases):
   - Generar archivo con formato que Siigo pueda importar
   - El usuario ejecuta la importación desde Siigo manualmente

2. **Exportación a Excel para importación manual**:
   - El middleware genera un archivo que el usuario carga en Siigo

3. **Escribir directamente en ISAM** (NO RECOMENDADO):
   - Alto riesgo de corrupción de datos
   - Requiere entender la estructura completa incluyendo índices

---

## Campos Clave para el Cruce

### NIT como identificador universal

El **NIT** (Número de Identificación Tributaria) es el campo que conecta ambos sistemas:

- En Finearom: `clients.nit`
- En Siigo: Campo dentro del registro Z17 (terceros)

Todos los cruces se hacen por NIT:
- Cliente en Finearom ↔ Tercero en Siigo
- Factura en Finearom ↔ Movimiento en Siigo (por NIT del cliente)
- Cartera en Siigo → Cartera en Finearom (por NIT)

### Código de producto

- En Finearom: `products.code`
- En Siigo: Código dentro del registro Z06

---

## Archivos ISAM Relevantes (Resumen Final)

### Prioridad 1 — Sincronizar inmediatamente
| Archivo | Contenido | Para qué |
|---------|-----------|----------|
| **Z17** | Terceros/Clientes | Sync clientes por NIT |
| **Z06** | Productos | Sync catálogo de productos |
| **Z49** | Movimientos | Detectar facturas y pagos |

### Prioridad 2 — Sincronizar después
| Archivo | Contenido | Para qué |
|---------|-----------|----------|
| **Z09YYYY** | Cartera | Importar cartera a Finearom |
| **Z03YYYY** | Mov. contables | Datos contables |
| **Z70** | Comprobantes | Seguimiento cobros |
| **Z90PO** | Pedidos Siigo | Cruzar con órdenes web |

### Prioridad 3 — Referencia
| Archivo | Contenido | Para qué |
|---------|-----------|----------|
| **ZDANE** | Ciudades | Validar ciudades |
| **ZIVA** | IVA | Tarifas impositivas |
| **Z06CP** | Precios | Historial de precios |
| **ENVIA/** | Despachos | Historial envíos |
| **C03** | Plan cuentas | Clasificación contable |

---

## Endpoints de Finearom para la Integración

### Para crear/actualizar clientes
```
POST /api/clients
PUT /api/clients/{clientId}
```
Campos mínimos: nit, client_name, email, phone, address, city

### Para crear/actualizar productos
```
POST /api/products
POST /api/products/import (bulk desde Excel)
PUT /api/products/{productId}
```
Campos mínimos: code, product_name, price

### Para importar cartera
```
POST /api/cartera/import (ya existe, acepta Excel)
```

### Para importar recaudos
```
POST /api/recaudo/import (ya existe, acepta Excel)
```

### Para crear órdenes (si se necesita en dirección inversa)
```
POST /api/purchase-orders
```

---

## Autenticación de la API

Finearom usa **Laravel Sanctum** (token-based):
```
POST /api/login → obtiene token
Authorization: Bearer {token}
```

El middleware necesita:
1. Un usuario de servicio en Finearom
2. Login al iniciar para obtener token
3. Usar el token en cada request
