export namespace goods {
	
	export class Item {
	    name: string;
	    price: number;
	    emoji: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.price = source["price"];
	        this.emoji = source["emoji"];
	    }
	}

}

export namespace main {
	
	export class Snapshot {
	    date: string;
	    clock: string;
	    weekday: string;
	    status: string;
	    statusTone: string;
	    untilOff: string;
	    untilOffSec: number;
	    untilStart: string;
	    worked: string;
	    totalWork: string;
	    progress: number;
	    earned: number;
	    dailyRate: number;
	    hourlyRate: number;
	    minuteRate: number;
	    hintEmoji: string;
	    hintItem: string;
	    hintPrice: number;
	    hintMinutes: number;
	    hintProgress: number;
	    hintText: string;
	    goods: goods.Item[];
	    openSettings: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.clock = source["clock"];
	        this.weekday = source["weekday"];
	        this.status = source["status"];
	        this.statusTone = source["statusTone"];
	        this.untilOff = source["untilOff"];
	        this.untilOffSec = source["untilOffSec"];
	        this.untilStart = source["untilStart"];
	        this.worked = source["worked"];
	        this.totalWork = source["totalWork"];
	        this.progress = source["progress"];
	        this.earned = source["earned"];
	        this.dailyRate = source["dailyRate"];
	        this.hourlyRate = source["hourlyRate"];
	        this.minuteRate = source["minuteRate"];
	        this.hintEmoji = source["hintEmoji"];
	        this.hintItem = source["hintItem"];
	        this.hintPrice = source["hintPrice"];
	        this.hintMinutes = source["hintMinutes"];
	        this.hintProgress = source["hintProgress"];
	        this.hintText = source["hintText"];
	        this.goods = this.convertValues(source["goods"], goods.Item);
	        this.openSettings = source["openSettings"];
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

export namespace settings {
	
	export class Settings {
	    annualSalary: number;
	    workStart: string;
	    lunchStart: string;
	    workResume: string;
	    workEnd: string;
	    alwaysOnTop: boolean;
	    theme: string;
	    customGoods: goods.Item[];
	    windowX: number;
	    windowY: number;
	    petEnabled: boolean;
	    petPack: string;
	    petX: number;
	    petY: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.annualSalary = source["annualSalary"];
	        this.workStart = source["workStart"];
	        this.lunchStart = source["lunchStart"];
	        this.workResume = source["workResume"];
	        this.workEnd = source["workEnd"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.theme = source["theme"];
	        this.customGoods = this.convertValues(source["customGoods"], goods.Item);
	        this.windowX = source["windowX"];
	        this.windowY = source["windowY"];
	        this.petEnabled = source["petEnabled"];
	        this.petPack = source["petPack"];
	        this.petX = source["petX"];
	        this.petY = source["petY"];
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

