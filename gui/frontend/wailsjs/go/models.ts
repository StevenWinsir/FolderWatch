export namespace backend {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    commit: string;
	    buildDate: string;
	    protocol: number;
	    heartbeatMillis: number;
	    leaseMillis: number;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.buildDate = source["buildDate"];
	        this.protocol = source["protocol"];
	        this.heartbeatMillis = source["heartbeatMillis"];
	        this.leaseMillis = source["leaseMillis"];
	    }
	}
	export class FileInfo {
	    sizeBytes: string;
	    kind: string;
	    classification: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sizeBytes = source["sizeBytes"];
	        this.kind = source["kind"];
	        this.classification = source["classification"];
	        this.reason = source["reason"];
	    }
	}
	export class ChangeSummary {
	    path: string;
	    oldPath: string;
	    kind: string;
	    version: string;
	    before?: FileInfo;
	    after?: FileInfo;
	    firstSeen: string;
	    lastSeen: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.oldPath = source["oldPath"];
	        this.kind = source["kind"];
	        this.version = source["version"];
	        this.before = this.convertValues(source["before"], FileInfo);
	        this.after = this.convertValues(source["after"], FileInfo);
	        this.firstSeen = source["firstSeen"];
	        this.lastSeen = source["lastSeen"];
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
	export class Problem {
	    code: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.message = source["message"];
	    }
	}
	export class ChangesReply {
	    sessionId: string;
	    generation: string;
	    version: string;
	    changes: ChangeSummary[];
	    total: number;
	    nextOffset: number;
	    error?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new ChangesReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.generation = source["generation"];
	        this.version = source["version"];
	        this.changes = this.convertValues(source["changes"], ChangeSummary);
	        this.total = source["total"];
	        this.nextOffset = source["nextOffset"];
	        this.error = this.convertValues(source["error"], Problem);
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
	export class ChangesRequest {
	    clientId: string;
	    sessionId: string;
	    offset: number;
	    limit: number;
	    generation: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.sessionId = source["sessionId"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	        this.generation = source["generation"];
	        this.version = source["version"];
	    }
	}
	export class SessionInfo {
	    sessionId: string;
	    root: string;
	    state: string;
	    sequence: string;
	    generation: string;
	    version: string;
	    warning: string;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.root = source["root"];
	        this.state = source["state"];
	        this.sequence = source["sequence"];
	        this.generation = source["generation"];
	        this.version = source["version"];
	        this.warning = source["warning"];
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class ConnectionReply {
	    clientId: string;
	    app: AppInfo;
	    status: SessionInfo;
	    error?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.app = this.convertValues(source["app"], AppInfo);
	        this.status = this.convertValues(source["status"], SessionInfo);
	        this.error = this.convertValues(source["error"], Problem);
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
	export class DiffLine {
	    kind: string;
	    oldLine: number;
	    newLine: number;
	    text: string;
	    noNewline: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.oldLine = source["oldLine"];
	        this.newLine = source["newLine"];
	        this.text = source["text"];
	        this.noNewline = source["noNewline"];
	    }
	}
	export class DiffHunk {
	    oldStart: number;
	    oldLines: number;
	    newStart: number;
	    newLines: number;
	    lines: DiffLine[];
	
	    static createFrom(source: any = {}) {
	        return new DiffHunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldStart = source["oldStart"];
	        this.oldLines = source["oldLines"];
	        this.newStart = source["newStart"];
	        this.newLines = source["newLines"];
	        this.lines = this.convertValues(source["lines"], DiffLine);
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
	
	export class DiffResult {
	    sessionId: string;
	    path: string;
	    kind: string;
	    status: string;
	    reason: string;
	    generation: string;
	    version: string;
	    hunks: DiffHunk[];
	
	    static createFrom(source: any = {}) {
	        return new DiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.generation = source["generation"];
	        this.version = source["version"];
	        this.hunks = this.convertValues(source["hunks"], DiffHunk);
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
	export class DiffReply {
	    diff?: DiffResult;
	    error?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new DiffReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.diff = this.convertValues(source["diff"], DiffResult);
	        this.error = this.convertValues(source["error"], Problem);
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
	export class DiffRequest {
	    clientId: string;
	    sessionId: string;
	    path: string;
	    generation: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.sessionId = source["sessionId"];
	        this.path = source["path"];
	        this.generation = source["generation"];
	        this.version = source["version"];
	    }
	}
	
	
	
	export class Reply {
	    status: SessionInfo;
	    error?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new Reply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = this.convertValues(source["status"], SessionInfo);
	        this.error = this.convertValues(source["error"], Problem);
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
	
	export class SessionRequest {
	    clientId: string;
	    sessionId: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.sessionId = source["sessionId"];
	    }
	}
	export class StartOptions {
	    clientId: string;
	    root: string;
	    debounce?: string;
	    ignore?: string[];
	    respectGitIgnore?: boolean;
	    maxDiffBytes?: string;
	
	    static createFrom(source: any = {}) {
	        return new StartOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.root = source["root"];
	        this.debounce = source["debounce"];
	        this.ignore = source["ignore"];
	        this.respectGitIgnore = source["respectGitIgnore"];
	        this.maxDiffBytes = source["maxDiffBytes"];
	    }
	}

	export class FolderReply {
	    path: string;
	    error?: Problem;

	    static createFrom(source: any = {}) {
	        return new FolderReply(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.error = this.convertValues(source["error"], Problem);
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

