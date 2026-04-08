export namespace config {
	
	export class SyncConfig {
	    interval_seconds: number;
	    files: string[];
	    state_path: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interval_seconds = source["interval_seconds"];
	        this.files = source["files"];
	        this.state_path = source["state_path"];
	    }
	}
	export class FinearomConfig {
	    base_url: string;
	    email: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new FinearomConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base_url = source["base_url"];
	        this.email = source["email"];
	        this.password = source["password"];
	    }
	}
	export class SiigoConfig {
	    data_path: string;
	
	    static createFrom(source: any = {}) {
	        return new SiigoConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data_path = source["data_path"];
	    }
	}
	export class Config {
	    siigo: SiigoConfig;
	    finearom: FinearomConfig;
	    sync: SyncConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.siigo = this.convertValues(source["siigo"], SiigoConfig);
	        this.finearom = this.convertValues(source["finearom"], FinearomConfig);
	        this.sync = this.convertValues(source["sync"], SyncConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace main {
	
	export class ISAMPreview {
	    file: string;
	    record_size: number;
	    records: number;
	    num_keys: number;
	    has_index: boolean;
	    used_extfh: boolean;
	    format: number;
	    mod_time: string;
	
	    static createFrom(source: any = {}) {
	        return new ISAMPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.record_size = source["record_size"];
	        this.records = source["records"];
	        this.num_keys = source["num_keys"];
	        this.has_index = source["has_index"];
	        this.used_extfh = source["used_extfh"];
	        this.format = source["format"];
	        this.mod_time = source["mod_time"];
	    }
	}
	export class PaginatedISAM {
	    data: any;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PaginatedISAM(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data = source["data"];
	        this.total = source["total"];
	    }
	}
	export class PaginatedLogs {
	    logs: storage.LogEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PaginatedLogs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.logs = this.convertValues(source["logs"], storage.LogEntry);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PaginatedRecords {
	    records: storage.SyncRecord[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PaginatedRecords(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], storage.SyncRecord);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace parsers {
	
	export class Cartera {
	    tipo_registro: string;
	    empresa: string;
	    secuencia: string;
	    tipo_doc: string;
	    nit_tercero: string;
	    cuenta_contable: string;
	    fecha: string;
	    descripcion: string;
	    tipo_mov: string;
	    hash: string;
	
	    static createFrom(source: any = {}) {
	        return new Cartera(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tipo_registro = source["tipo_registro"];
	        this.empresa = source["empresa"];
	        this.secuencia = source["secuencia"];
	        this.tipo_doc = source["tipo_doc"];
	        this.nit_tercero = source["nit_tercero"];
	        this.cuenta_contable = source["cuenta_contable"];
	        this.fecha = source["fecha"];
	        this.descripcion = source["descripcion"];
	        this.tipo_mov = source["tipo_mov"];
	        this.hash = source["hash"];
	    }
	}
	export class Movimiento {
	    tipo_comprobante: string;
	    empresa: string;
	    numero_doc: string;
	    fecha: string;
	    nit_tercero: string;
	    cuenta_contable: string;
	    descripcion: string;
	    valor: string;
	    tipo_mov: string;
	    raw_preview: string;
	    hash: string;
	
	    static createFrom(source: any = {}) {
	        return new Movimiento(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tipo_comprobante = source["tipo_comprobante"];
	        this.empresa = source["empresa"];
	        this.numero_doc = source["numero_doc"];
	        this.fecha = source["fecha"];
	        this.nit_tercero = source["nit_tercero"];
	        this.cuenta_contable = source["cuenta_contable"];
	        this.descripcion = source["descripcion"];
	        this.valor = source["valor"];
	        this.tipo_mov = source["tipo_mov"];
	        this.raw_preview = source["raw_preview"];
	        this.hash = source["hash"];
	    }
	}
	export class Producto {
	    comprobante: string;
	    secuencia: string;
	    tipo_tercero: string;
	    grupo: string;
	    cuenta_contable?: string;
	    fecha?: string;
	    nombre: string;
	    tipo_mov?: string;
	    hash: string;
	
	    static createFrom(source: any = {}) {
	        return new Producto(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comprobante = source["comprobante"];
	        this.secuencia = source["secuencia"];
	        this.tipo_tercero = source["tipo_tercero"];
	        this.grupo = source["grupo"];
	        this.cuenta_contable = source["cuenta_contable"];
	        this.fecha = source["fecha"];
	        this.nombre = source["nombre"];
	        this.tipo_mov = source["tipo_mov"];
	        this.hash = source["hash"];
	    }
	}
	export class Tercero {
	    tipo_clave: string;
	    empresa: string;
	    codigo: string;
	    tipo_doc: string;
	    numero_doc: string;
	    fecha_creacion: string;
	    nombre: string;
	    tipo_cta_pref: string;
	    hash: string;
	
	    static createFrom(source: any = {}) {
	        return new Tercero(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tipo_clave = source["tipo_clave"];
	        this.empresa = source["empresa"];
	        this.codigo = source["codigo"];
	        this.tipo_doc = source["tipo_doc"];
	        this.numero_doc = source["numero_doc"];
	        this.fecha_creacion = source["fecha_creacion"];
	        this.nombre = source["nombre"];
	        this.tipo_cta_pref = source["tipo_cta_pref"];
	        this.hash = source["hash"];
	    }
	}

}

export namespace storage {
	
	export class LogEntry {
	    id: number;
	    level: string;
	    source: string;
	    message: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.created_at = source["created_at"];
	    }
	}
	export class SyncRecord {
	    id: number;
	    table: string;
	    key: string;
	    data: string;
	    hash: string;
	    sync_status: string;
	    sync_error: string;
	    sync_action: string;
	    updated_at: string;
	    synced_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.table = source["table"];
	        this.key = source["key"];
	        this.data = source["data"];
	        this.hash = source["hash"];
	        this.sync_status = source["sync_status"];
	        this.sync_error = source["sync_error"];
	        this.sync_action = source["sync_action"];
	        this.updated_at = source["updated_at"];
	        this.synced_at = source["synced_at"];
	    }
	}

}

