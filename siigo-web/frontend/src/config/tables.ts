// tables.ts — Column definitions for all data tables.
// DataPage reads this config to render headers and cells generically.

export type ColType = 'key' | 'name' | 'date' | 'code' | 'type' | 'desc' | 'value' | 'bool';

export interface ColDef {
  field: string;    // JSON field name from API
  label: string;    // Header label
  type: ColType;    // CSS class suffix (col-key, col-name, etc.)
  bold?: boolean;   // Extra font-weight (e.g. saldo_final)
  format?: 'money' | 'percent'; // money: toLocaleString with 2 decimals, percent: N.NNN%
}

export interface TableConfig {
  columns: ColDef[];
  editableFields: string[];
  apiPath: string;  // URL path segment (e.g. "clients", "plan-cuentas")
}

const money = 'money' as const;

export const TABLE_CONFIGS: Record<string, TableConfig> = {
  clients: {
    apiPath: 'clients',
    editableFields: ['nit', 'nombre', 'tipo_persona', 'empresa', 'direccion', 'email', 'rep_legal'],
    columns: [
      { field: 'nit', label: 'NIT', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'tipo_persona', label: 'Tipo', type: 'type' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'email', label: 'Email', type: 'desc' },
    ],
  },
  products: {
    apiPath: 'products',
    editableFields: ['code', 'nombre', 'grupo', 'cuenta_contable', 'fecha', 'tipo_mov'],
    columns: [
      { field: 'code', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'grupo', label: 'Grupo', type: 'type' },
      { field: 'cuenta_contable', label: 'Cuenta', type: 'code' },
    ],
  },
  movements: {
    apiPath: 'movements',
    editableFields: ['tipo_comprobante', 'numero_doc', 'fecha', 'nit_tercero', 'cuenta_contable', 'descripcion', 'valor', 'tipo_mov'],
    columns: [
      { field: 'tipo_comprobante', label: 'Tipo', type: 'type' },
      { field: 'numero_doc', label: 'Num Doc', type: 'key' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'nit_tercero', label: 'NIT', type: 'code' },
      { field: 'descripcion', label: 'Descripcion', type: 'desc' },
    ],
  },
  cartera: {
    apiPath: 'cartera',
    editableFields: ['tipo_registro', 'nit_tercero', 'cuenta_contable', 'fecha', 'descripcion', 'tipo_mov'],
    columns: [
      { field: 'tipo_registro', label: 'Tipo', type: 'type' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'cuenta_contable', label: 'Cuenta', type: 'code' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'descripcion', label: 'Descripcion', type: 'desc' },
      { field: 'tipo_mov', label: 'D/C', type: 'value' },
    ],
  },
  plan_cuentas: {
    apiPath: 'plan-cuentas',
    editableFields: ['codigo_cuenta', 'nombre', 'empresa', 'naturaleza'],
    columns: [
      { field: 'codigo_cuenta', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'activa', label: 'Activa', type: 'bool' },
      { field: 'auxiliar', label: 'Auxiliar', type: 'bool' },
      { field: 'naturaleza', label: 'Naturaleza', type: 'desc' },
    ],
  },
  activos_fijos: {
    apiPath: 'activos-fijos',
    editableFields: ['codigo', 'nombre', 'empresa', 'nit_responsable', 'fecha_adquisicion'],
    columns: [
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'nit_responsable', label: 'NIT Responsable', type: 'code' },
      { field: 'fecha_adquisicion', label: 'Fecha Adquisicion', type: 'date' },
    ],
  },
  saldos_terceros: {
    apiPath: 'saldos-terceros',
    editableFields: ['cuenta_contable', 'nit_tercero'],
    columns: [
      { field: 'cuenta_contable', label: 'Cuenta', type: 'code' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'saldo_anterior', label: 'Saldo Ant.', type: 'value', format: money },
      { field: 'debito', label: 'Debito', type: 'value', format: money },
      { field: 'credito', label: 'Credito', type: 'value', format: money },
      { field: 'saldo_final', label: 'Saldo Final', type: 'value', format: money, bold: true },
    ],
  },
  saldos_consolidados: {
    apiPath: 'saldos-consolidados',
    editableFields: ['cuenta_contable'],
    columns: [
      { field: 'cuenta_contable', label: 'Cuenta', type: 'key' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'saldo_anterior', label: 'Saldo Ant.', type: 'value', format: money },
      { field: 'debito', label: 'Debito', type: 'value', format: money },
      { field: 'credito', label: 'Credito', type: 'value', format: money },
      { field: 'saldo_final', label: 'Saldo Final', type: 'value', format: money, bold: true },
    ],
  },
  documentos: {
    apiPath: 'documentos',
    editableFields: ['tipo_comprobante', 'nit_tercero', 'cuenta_contable', 'producto_ref', 'fecha', 'descripcion', 'tipo_mov'],
    columns: [
      { field: 'tipo_comprobante', label: 'Tipo', type: 'type' },
      { field: 'codigo_comp', label: 'Cod', type: 'code' },
      { field: 'secuencia', label: 'Seq', type: 'code' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'cuenta_contable', label: 'Cuenta', type: 'code' },
      { field: 'producto_ref', label: 'Producto', type: 'code' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'descripcion', label: 'Descripcion', type: 'desc' },
      { field: 'tipo_mov', label: 'D/C', type: 'value' },
    ],
  },
  terceros_ampliados: {
    apiPath: 'terceros-ampliados',
    editableFields: ['nit', 'nombre', 'tipo_persona', 'representante_legal', 'direccion', 'email'],
    columns: [
      { field: 'nit', label: 'NIT', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'tipo_persona', label: 'Tipo', type: 'type' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'representante_legal', label: 'Rep. Legal', type: 'name' },
      { field: 'direccion', label: 'Direccion', type: 'desc' },
      { field: 'email', label: 'Email', type: 'desc' },
    ],
  },
  movimientos_inventario: {
    apiPath: 'movimientos-inventario',
    editableFields: ['codigo_producto', 'tipo_comprobante', 'fecha', 'cantidad', 'valor', 'tipo_mov'],
    columns: [
      { field: 'codigo_producto', label: 'Producto', type: 'key' },
      { field: 'tipo_comprobante', label: 'Tipo', type: 'type' },
      { field: 'codigo_comp', label: 'Comp', type: 'code' },
      { field: 'secuencia', label: 'Seq', type: 'code' },
      { field: 'tipo_doc', label: 'TipoDoc', type: 'code' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'cantidad', label: 'Cantidad', type: 'value' },
      { field: 'valor', label: 'Valor', type: 'value' },
      { field: 'tipo_mov', label: 'D/C', type: 'type' },
    ],
  },
  saldos_inventario: {
    apiPath: 'saldos-inventario',
    editableFields: ['codigo_producto'],
    columns: [
      { field: 'codigo_producto', label: 'Producto', type: 'key' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'grupo', label: 'Grupo', type: 'code' },
      { field: 'saldo_inicial', label: 'Saldo Ini.', type: 'value', format: money },
      { field: 'entradas', label: 'Entradas', type: 'value', format: money },
      { field: 'salidas', label: 'Salidas', type: 'value', format: money },
      { field: 'saldo_final', label: 'Saldo Final', type: 'value', format: money, bold: true },
    ],
  },
  activos_fijos_detalle: {
    apiPath: 'activos-fijos-detalle',
    editableFields: ['nombre', 'nit_responsable', 'codigo', 'fecha', 'valor_compra', 'ubicacion', 'referencia'],
    columns: [
      { field: 'codigo', label: 'Codigo', type: 'code' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'nit_responsable', label: 'NIT Resp', type: 'key' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'valor_compra', label: 'Valor Compra', type: 'value', format: money },
      { field: 'ubicacion', label: 'Ubicacion', type: 'desc' },
      { field: 'referencia', label: 'Referencia', type: 'code' },
    ],
  },
  audit_trail_terceros: {
    apiPath: 'audit-trail-terceros',
    editableFields: ['nombre', 'nit_tercero', 'fecha_cambio', 'tipo_doc', 'direccion', 'email'],
    columns: [
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'fecha_cambio', label: 'Fecha', type: 'date' },
      { field: 'tipo_doc', label: 'Tipo', type: 'type' },
      { field: 'usuario', label: 'Usuario', type: 'code' },
      { field: 'direccion', label: 'Direccion', type: 'desc' },
      { field: 'email', label: 'Email', type: 'desc' },
      { field: 'rep_legal', label: 'Rep. Legal', type: 'name' },
    ],
  },
  transacciones_detalle: {
    apiPath: 'transacciones-detalle',
    editableFields: ['nit_tercero', 'cuenta_contable', 'fecha_documento', 'valor', 'tipo_movimiento'],
    columns: [
      { field: 'tipo_comprobante', label: 'Tipo Comp', type: 'type' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'cuenta_contable', label: 'Cuenta', type: 'code' },
      { field: 'fecha_documento', label: 'Fecha', type: 'date' },
      { field: 'tipo_movimiento', label: 'D/C', type: 'type' },
      { field: 'valor', label: 'Valor', type: 'value' },
      { field: 'referencia', label: 'Referencia', type: 'code' },
    ],
  },
  periodos_contables: {
    apiPath: 'periodos-contables',
    editableFields: ['numero_periodo', 'fecha_inicio', 'fecha_fin', 'estado'],
    columns: [
      { field: 'numero_periodo', label: 'Periodo', type: 'key' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'fecha_inicio', label: 'Fecha Inicio', type: 'date' },
      { field: 'fecha_fin', label: 'Fecha Fin', type: 'date' },
      { field: 'estado', label: 'Estado Periodo', type: 'type' },
      { field: 'saldo1', label: 'Saldo 1', type: 'value', format: money },
      { field: 'saldo2', label: 'Saldo 2', type: 'value', format: money },
      { field: 'saldo3', label: 'Saldo 3', type: 'value', format: money },
    ],
  },
  condiciones_pago: {
    apiPath: 'condiciones-pago',
    editableFields: ['tipo', 'nit', 'fecha', 'valor', 'fecha_registro'],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'nit', label: 'NIT', type: 'key' },
      { field: 'tipo_doc', label: 'TipoDoc', type: 'type' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'valor', label: 'Valor', type: 'value', format: money },
      { field: 'fecha_registro', label: 'Fecha Reg', type: 'date' },
    ],
  },
  libros_auxiliares: {
    apiPath: 'libros-auxiliares',
    editableFields: ['cuenta_contable', 'nit_tercero', 'fecha_documento', 'saldo', 'debito', 'credito'],
    columns: [
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'cuenta_contable', label: 'Cuenta', type: 'key' },
      { field: 'tipo_comprobante', label: 'TipoComp', type: 'type' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'fecha_documento', label: 'Fecha', type: 'date' },
      { field: 'saldo', label: 'Saldo', type: 'value', format: money },
      { field: 'debito', label: 'Debito', type: 'value', format: money },
      { field: 'credito', label: 'Credito', type: 'value', format: money },
    ],
  },
  codigos_dane: {
    apiPath: 'codigos-dane',
    editableFields: ['codigo', 'nombre'],
    columns: [
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
    ],
  },
  actividades_ica: {
    apiPath: 'actividades-ica',
    editableFields: ['codigo', 'nombre', 'tarifa'],
    columns: [
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'tarifa', label: 'Tarifa', type: 'value' },
    ],
  },
  conceptos_pila: {
    apiPath: 'conceptos-pila',
    editableFields: ['tipo', 'fondo', 'concepto'],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'fondo', label: 'Fondo', type: 'code' },
      { field: 'concepto', label: 'Concepto', type: 'key' },
      { field: 'flags', label: 'Flags', type: 'code' },
      { field: 'tipo_base', label: 'Tipo Base', type: 'type' },
      { field: 'base_calculo', label: 'Base Calculo', type: 'value' },
    ],
  },
  clasificacion_cuentas: {
    apiPath: 'clasificacion-cuentas',
    editableFields: ['codigo_cuenta', 'descripcion'],
    columns: [
      { field: 'codigo_cuenta', label: 'Cuenta', type: 'key' },
      { field: 'codigo_grupo', label: 'Grupo', type: 'code' },
      { field: 'codigo_detalle', label: 'Detalle', type: 'code' },
      { field: 'descripcion', label: 'Descripcion', type: 'name' },
    ],
  },
  historial: {
    apiPath: 'historial',
    editableFields: ['tipo', 'sub_tipo', 'empresa', 'fecha', 'nombre', 'nombre2'],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'sub_tipo', label: 'SubTipo', type: 'code' },
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'nombre2', label: 'Nombre2', type: 'name' },
    ],
  },
  maestros: {
    apiPath: 'maestros',
    editableFields: ['tipo', 'codigo', 'nombre', 'responsable', 'direccion'],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'responsable', label: 'Responsable', type: 'name' },
      { field: 'direccion', label: 'Direccion', type: 'desc' },
    ],
  },
  formulas: {
    apiPath: 'formulas',
    editableFields: ['empresa', 'codigo_producto', 'grupo_ingrediente', 'codigo_ingrediente', 'porcentaje'],
    columns: [
      { field: 'empresa', label: 'Empresa', type: 'code' },
      { field: 'grupo_producto', label: 'Grupo Prod.', type: 'code' },
      { field: 'codigo_producto', label: 'Producto', type: 'key' },
      { field: 'grupo_ingrediente', label: 'Grupo Ing.', type: 'code' },
      { field: 'codigo_ingrediente', label: 'Ingrediente', type: 'key' },
      { field: 'porcentaje', label: '% Composicion', type: 'value', format: 'percent' },
    ],
  },
  docs_inventario: {
    apiPath: 'docs-inventario',
    editableFields: [],
    columns: [
      { field: 'tipo_doc', label: 'Tipo Doc', type: 'type' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'usuario_crea', label: 'Usuario', type: 'name' },
      { field: 'codigo_producto', label: 'Producto', type: 'key' },
      { field: 'campo_modificado', label: 'Campo', type: 'desc' },
      { field: 'modulo_origen', label: 'Modulo', type: 'code' },
    ],
  },
  vendedores_areas: {
    apiPath: 'vendedores-areas',
    editableFields: ['nombre', 'nombre_corto', 'ciudad', 'nit', 'direccion', 'email'],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'ciudad', label: 'Ciudad', type: 'desc' },
      { field: 'nit', label: 'NIT', type: 'key' },
      { field: 'email', label: 'Email', type: 'desc' },
    ],
  },
};
