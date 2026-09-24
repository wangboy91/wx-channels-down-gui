export namespace main {
	
	export class LogEntry {
	    time: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}
	export class Settings {
	    proxyPort: number;
	    apiPort: number;
	    downloadDir: string;
	    upstreamProxy: string;
	    maxRunning: number;
	    defaultHighest: boolean;
	    downloadCover: boolean;
	    playDoneAudio: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proxyPort = source["proxyPort"];
	        this.apiPort = source["apiPort"];
	        this.downloadDir = source["downloadDir"];
	        this.upstreamProxy = source["upstreamProxy"];
	        this.maxRunning = source["maxRunning"];
	        this.defaultHighest = source["defaultHighest"];
	        this.downloadCover = source["downloadCover"];
	        this.playDoneAudio = source["playDoneAudio"];
	    }
	}
	export class Status {
	    running: boolean;
	    proxyPort: number;
	    apiPort: number;
	    uptime: string;
	    downloadDir: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.proxyPort = source["proxyPort"];
	        this.apiPort = source["apiPort"];
	        this.uptime = source["uptime"];
	        this.downloadDir = source["downloadDir"];
	    }
	}

}

