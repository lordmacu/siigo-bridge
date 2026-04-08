========================================
  SIIGO WEB - Middleware de Datos
========================================

Siigo Web es un middleware que conecta Siigo Pyme (software contable COBOL)
con sistemas externos, permitiendo consultar y sincronizar datos contables
en tiempo real a traves de una interfaz web moderna.


QUE HACE
--------
- Lee los archivos ISAM de Siigo Pyme directamente (sin modificarlos)
- Detecta cambios automaticamente cuando Siigo escribe datos
- Presenta los datos en un panel web accesible desde cualquier navegador
- Sincroniza datos con plataformas externas via API REST
- Expone una API publica (REST + OData) para integracion con Power BI,
  Excel, y otros sistemas


TABLAS DISPONIBLES (27)
-----------------------
Clientes, Productos, Movimientos, Cartera, Plan de Cuentas, Activos Fijos,
Saldos por Tercero, Saldos Consolidados, Documentos, Terceros Ampliados,
Transacciones Detalle, Periodos Contables, Condiciones de Pago,
Libros Auxiliares, Codigos DANE, Actividades ICA, Conceptos PILA,
Activos Fijos Detalle, Audit Trail Terceros, Clasificacion de Cuentas,
Movimientos Inventario, Saldos Inventario, Historial, Maestros,
Formulas/Recetas, Docs Inventario, Vendedores/Areas


COMO ACCEDER
------------
1. Abra un navegador web (Chrome, Edge, Firefox)
2. Ingrese a:  http://localhost:PUERTO  (ej: http://localhost:3210)
3. Use las credenciales configuradas durante la instalacion
4. Para acceso desde otros equipos en la red, use la IP del servidor:
   http://IP-DEL-EQUIPO:PUERTO


FUNCIONALIDADES
---------------
- Dashboard con estadisticas y graficos
- Consulta de datos por tabla con busqueda y paginacion
- SQL Explorer para consultas personalizadas
- API REST publica con documentacion Swagger
- OData v4 compatible con Power BI
- Sincronizacion automatica con deteccion de cambios en tiempo real
- Notificaciones y control remoto via Telegram Bot
- Exportacion de coleccion Postman
- Sistema de usuarios con roles y permisos por modulo


API PUBLICA
-----------
Documentacion interactiva (Swagger): http://localhost:PUERTO/api/v1/docs

Endpoints principales:
  POST /api/v1/auth          Obtener token JWT
  GET  /api/v1/stats         Estadisticas generales
  GET  /api/v1/{tabla}       Listar registros (paginado, con busqueda)
  GET  /api/v1/{tabla}/{key} Detalle por clave

OData (Power BI):
  GET  /odata                Documento de servicio
  GET  /odata/$metadata      Esquema XML
  GET  /odata/{tabla}        Consulta con $top, $skip, $filter, $orderby


CONFIGURACION
-------------
La configuracion se guarda en config.json junto al ejecutable.
Puede modificarla desde el panel web en la seccion Configuracion.

Opciones configurables:
  - Puerto del servidor
  - Ruta de datos de Siigo
  - Conexion a API externa (Finearom)
  - Intervalos de sincronizacion
  - Telegram Bot (notificaciones y control remoto)
  - API publica (activar/desactivar, JWT)
  - Permisos por usuario y modulo


SOPORTE
-------
Para soporte tecnico, contacte al administrador del sistema.


========================================
  Desarrollado para Finearom S.A.S.
========================================
