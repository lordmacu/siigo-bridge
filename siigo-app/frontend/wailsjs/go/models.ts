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
	    mod_time: string;
	
	    static createFrom(source: any = {}) {
	        return new ISAMPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.record_size = source["record_size"];
	        this.records = source["records"];
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
	    records: storage.SentRecord[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new PaginatedRecords(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], storage.SentRecord);
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
	export class SentRecord {
	    id: number;
	    table: string;
	    source_file: string;
	    key: string;
	    data: string;
	    status: string;
	    error: string;
	    hash: string;
	    sent_at: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SentRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.table = source["table"];
	        this.source_file = source["source_file"];
	        this.key = source["key"];
	        this.data = source["data"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.hash = source["hash"];
	        this.sent_at = source["sent_at"];
	        this.created_at = source["created_at"];
	    }
	}

}

