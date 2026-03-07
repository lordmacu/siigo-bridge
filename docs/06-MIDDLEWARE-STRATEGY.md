# Siigo - Estrategia para el Middleware

## Objetivo

Crear un servicio que detecte cambios en Siigo y los envíe a una plataforma web de órdenes de compra.

## Dirección del flujo

```
Siigo (escritorio) ──────→ Middleware (servicio) ──────→ Plataforma Web
     archivos ISAM          polling + lectura              API HTTP
                            detección cambios              POST/PUT
```

**Unidireccional**: Solo lectura de Siigo → Web. No escribimos en los archivos ISAM (riesgo de corrupción).

## Arquitectura Propuesta

```
┌─────────────────────────────────────────────────────┐
│                 Worker Service (.NET)                 │
│                                                       │
│  ┌───────────────┐  ┌──────────────┐  ┌───────────┐ │
│  │  ISAM Reader   │  │  Change      │  │  HTTP     │ │
│  │  (lee archivos │→ │  Detector    │→ │  Sender   │ │
│  │   binarios)    │  │  (compara)   │  │  (envía)  │ │
│  └───────────────┘  └──────────────┘  └───────────┘ │
│         ↑                    ↑                        │
│    C:\DEMOS01\*        Estado local                   │
│                      (JSON/SQLite)                    │
└─────────────────────────────────────────────────────┘
```

### Componentes

1. **ISAM Reader** — Lee archivos ISAM binarios (ya funcional, ver SiigoExplorer)
2. **Change Detector** — Compara timestamps de archivos + hashes de registros con estado previo
3. **HTTP Sender** — Envía datos nuevos/modificados a la API web
4. **Estado Local** — Almacena último estado conocido (SQLite o JSON)

## Detección de Cambios

### Estrategia: Polling por timestamp + hash de contenido

```
Cada N minutos:
  1. Verificar timestamp de última modificación de cada archivo ISAM
  2. Si el timestamp cambió → re-leer el archivo
  3. Comparar registros con el último estado conocido (por hash)
  4. Registros nuevos o modificados → enviar a la web
  5. Actualizar estado local
```

### Ventajas
- No interfiere con Siigo (solo lectura)
- Funciona con archivos bloqueados (FileShare.ReadWrite)
- Detecta cualquier cambio en los datos

### Desventajas
- Hay un delay entre el cambio en Siigo y la detección (depende del intervalo de polling)
- Lee archivos completos cada vez que hay un cambio

## Datos Relevantes para Órdenes de Compra

### Prioridad Alta
| Archivo | Datos | Para qué |
|---------|-------|----------|
| **Z17** | Terceros (proveedores, clientes) | Quién compra/vende |
| **Z06** | Productos/inventario | Qué se compra |
| **Z49** | Movimientos/transacciones | Las compras registradas |

### Prioridad Media
| Archivo | Datos | Para qué |
|---------|-------|----------|
| **C03** | Plan de cuentas | Clasificación contable |
| **Z70** | Comprobantes | Seguimiento de cobros |
| **Z09YYYY** | Cartera | Estado de cuentas |

### Prioridad Baja (referencia)
| Archivo | Datos | Para qué |
|---------|-------|----------|
| **ZDANE** | Códigos ciudades | Validación de direcciones |
| **ZIVA** | Tarifas IVA | Cálculo de impuestos |
| **Z90ES** | Módulos/permisos | Referencia |

## Formato de Datos para la API

### Tercero (Proveedor/Cliente)
```json
{
  "source": "siigo",
  "type": "tercero",
  "key": "G00100000000002001",
  "data": {
    "tipoDocumento": "NIT",
    "numeroDocumento": "3005000020",
    "nombre": "PROVEEDORES",
    "direccion": "",
    "ciudad": "",
    "telefono": "",
    "fechaCreacion": "20121030",
    "estado": "activo"
  },
  "timestamp": "2026-03-06T16:43:00Z",
  "hash": "sha256..."
}
```

### Producto
```json
{
  "source": "siigo",
  "type": "producto",
  "key": "PROD001",
  "data": {
    "codigo": "PROD001",
    "nombre": "ELEVIDOR LD 32 PULGADAS",
    "unidadMedida": "UND",
    "grupoInventario": "01",
    "precioVenta": 1500000,
    "iva": 19,
    "estado": "activo"
  },
  "timestamp": "2026-03-06T16:43:00Z",
  "hash": "sha256..."
}
```

### Movimiento/Transacción
```json
{
  "source": "siigo",
  "type": "movimiento",
  "key": "MOV-2014-00001",
  "data": {
    "tipoComprobante": "FV",
    "numero": "00001",
    "fecha": "20140725",
    "terceroNit": "0000830045678000",
    "cuenta": "4135",
    "debito": 0,
    "credito": 1500000,
    "descripcion": "ABONO A FACT. CTA 01.",
    "referencia": ""
  },
  "timestamp": "2026-03-06T16:43:00Z",
  "hash": "sha256..."
}
```

## Tecnologías Sugeridas

| Componente | Tecnología | Razón |
|-----------|------------|-------|
| Servicio | .NET Worker Service | Compatible con las DLLs de Siigo (.NET 4.8, x86) |
| Estado local | SQLite | Ligero, sin servidor, embebido |
| HTTP Client | HttpClient | Nativo en .NET |
| Scheduling | Timer/BackgroundService | Simple polling |
| Logging | Serilog o NLog | Para diagnosticar problemas |
| Configuración | appsettings.json | Estándar .NET |

## Configuración del Servicio

```json
{
  "Siigo": {
    "DataPath": "C:\\DEMOS01\\",
    "PollingIntervalSeconds": 60,
    "FilesToWatch": ["Z17", "Z06", "Z49", "Z70"]
  },
  "WebApi": {
    "BaseUrl": "https://tu-plataforma.com/api",
    "ApiKey": "xxx",
    "Timeout": 30
  }
}
```

## Consideraciones

### Seguridad
- El servicio corre en la misma máquina que Siigo (acceso local a archivos)
- La comunicación con la web debe ser HTTPS
- API key o OAuth para autenticación con la plataforma web

### Rendimiento
- Los archivos ISAM son pequeños (máximo 7.5MB) — la lectura es rápida
- Polling cada 1-5 minutos es suficiente para la mayoría de casos
- Almacenar hashes de registros evita re-enviar datos sin cambios

### Robustez
- Manejar archivos bloqueados (FileShare.ReadWrite)
- Reintentos con backoff exponencial para envíos HTTP fallidos
- Cola de mensajes local si la web no está disponible
- Logging detallado para diagnóstico

### Instalación
- Instalar como Windows Service (sc create)
- O como tarea programada (Task Scheduler)
- Debe correr en la misma máquina que Siigo

## Pasos Siguientes

1. **Refinar el lector ISAM** — Parsear campos específicos de Z17, Z06, Z49
2. **Crear el detector de cambios** — Polling + comparación por hash
3. **Definir la API web** — Endpoints, formato, autenticación
4. **Construir el servicio** — Worker Service con los 3 componentes
5. **Probar con datos demo** — Verificar lectura correcta con Siigo abierto
6. **Desplegar** — Instalar como servicio en la máquina del cliente
