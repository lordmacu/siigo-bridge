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
    editableFields: ['nit', 'nombre', 'tipo_persona', 'direccion', 'email'],
    columns: [
      { field: 'nit', label: 'NIT', type: 'key' },
      { field: 'dv', label: 'DV', type: 'code' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'tipo_persona', label: 'Tipo', type: 'type' },
      { field: 'direccion', label: 'Direccion', type: 'desc' },
      { field: 'email', label: 'Email', type: 'desc' },
    ],
  },
  products: {
    apiPath: 'products',
    editableFields: ['code', 'nombre', 'grupo'],
    columns: [
      { field: 'code', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'codigo_plataforma', label: 'Cod Plataforma', type: 'key', bold: true },
      { field: 'grupo', label: 'Grupo', type: 'type' },
    ],
  },
  cartera: {
    apiPath: 'cartera',
    editableFields: ['tipo_registro', 'nit_tercero', 'cuenta_contable', 'fecha', 'descripcion', 'tipo_mov'],
    columns: [
      { field: 'tipo_registro', label: 'Tipo', type: 'type' },
      { field: 'documento_ref', label: 'Documento', type: 'code', bold: true },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'descripcion', label: 'Descripcion', type: 'desc' },
      { field: 'codigo_plataforma', label: 'Cod Producto', type: 'key' },
      { field: 'valor', label: 'Valor', type: 'value', format: money, bold: true },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'tipo_mov', label: 'D/C', type: 'value' },
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
  codigos_dane: {
    apiPath: 'codigos-dane',
    editableFields: ['codigo', 'nombre'],
    columns: [
      { field: 'codigo', label: 'Codigo', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
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
  notas_documentos: {
    apiPath: 'notas-documentos',
    editableFields: [],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'codigo_doc', label: 'Cod', type: 'code' },
      { field: 'num_documento', label: 'Num Doc', type: 'key' },
      { field: 'orden_compra', label: 'Orden Compra', type: 'key', bold: true },
      { field: 'lote', label: 'Lote', type: 'code' },
      { field: 'fecha_despacho', label: 'Fecha Despacho', type: 'date' },
      { field: 'empaque', label: 'Empaque', type: 'desc' },
    ],
  },
  facturas_electronicas: {
    apiPath: 'facturas-electronicas',
    editableFields: [],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'nit_tercero', label: 'NIT', type: 'key' },
      { field: 'descripcion', label: 'Producto', type: 'name' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'valor', label: 'Valor', type: 'value', format: money },
      { field: 'tipo_mov', label: 'D/C', type: 'value' },
      { field: 'vendedor', label: 'Vendedor', type: 'name' },
    ],
  },
  detalle_movimientos: {
    apiPath: 'detalle-movimientos',
    editableFields: [],
    columns: [
      { field: 'tipo', label: 'Tipo', type: 'type' },
      { field: 'codigo_producto', label: 'Producto', type: 'key' },
      { field: 'nombre', label: 'Nombre', type: 'name' },
      { field: 'tipo_comprobante', label: 'Comp', type: 'code' },
      { field: 'num_comprobante', label: 'Num', type: 'code' },
      { field: 'bodega', label: 'Bodega', type: 'code' },
      { field: 'fecha', label: 'Fecha', type: 'date' },
      { field: 'valor', label: 'Valor', type: 'value', format: money },
      { field: 'tipo_mov', label: 'D/C', type: 'value' },
    ],
  },
};
