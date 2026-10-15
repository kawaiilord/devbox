const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function fixture({firstByteDelay=0}={}){
 class Media { get src(){return this.value||''} set src(v){this.value=v} }
 const video=new Media();let loads=[];let destroyed=0;
 class Player { static isBrowserSupported(){return true} async attach(v){this.video=v} configure(value){this.config=value} addEventListener(){} async load(url,_start,mime){if(firstByteDelay>(this.config.manifest?.retryParameters?.connectionTimeout??10000))throw Error('connection deadline');loads.push({url,mime});this.video.src='blob:media'} async destroy(){destroyed++} }
 const global={HTMLMediaElement:Media,URL,document:{baseURI:'https://app.example/'},Map,Date,Promise,Error,setTimeout,Hls:{isSupported:()=>true},shaka:{polyfill:{installAll(){}},Player}};
 global['$com.alexmercerind.media_kit.instances']={'7':video};global.globalThis=global;vm.createContext(global);vm.runInContext(fs.readFileSync(require.resolve('../web/adaptive-media.js'),'utf8'),global);
 return {global,video,loads,get destroyed(){return destroyed}};
}
test('DASH loads through the adapter and leaves unrelated native sources intact',async()=>{
 const f=fixture();const errors=[];
 await f.global.sameframeAdaptive.prepare(7,'https://app.example/media/ticket/stream.mpd',false,message=>errors.push(message));
 f.video.src='';f.video.src='https://app.example/media/ticket/stream.mpd';await f.global.sameframeAdaptive.ready(7);
 assert.equal(f.loads.length,1);assert.equal(f.loads[0].mime,'application/dash+xml');assert.equal(f.video.src,'blob:media');assert.deepEqual(errors,[]);
 await f.global.sameframeAdaptive.prepare(7,'https://app.example/movie.mp4',false,()=>{});
 f.video.src='https://app.example/movie.mp4';assert.equal(f.video.src,'https://app.example/movie.mp4');assert.equal(f.destroyed,1);
});
test('switching HLS destroys the old engine before the next source',async()=>{
 const f=fixture();for(const name of ['first','second']){const url='https://app.example/'+name+'.m3u8';await f.global.sameframeAdaptive.prepare(7,url,false,()=>{});f.video.src=url;await f.global.sameframeAdaptive.ready(7)}
 assert.equal(f.loads.length,2);assert.equal(f.loads[1].mime,'application/x-mpegurl');assert.equal(f.destroyed,1);
 await f.global.sameframeAdaptive.dispose(7);assert.equal(f.destroyed,2);assert.equal(Object.hasOwn(f.video,'src'),false);
});
test('a cold private HLS provider can prepare media beyond the default connection deadline',async()=>{
 const f=fixture({firstByteDelay:23000}),errors=[];
 const url='https://app.example/media/ticket/stream.m3u8';
 await f.global.sameframeAdaptive.prepare(7,url,false,message=>errors.push(message));
 f.video.src=url;await f.global.sameframeAdaptive.ready(7);
 assert.equal(f.loads.length,1);assert.deepEqual(errors,[]);
 // The longer preparation budget is confined to our authenticated media proxy.
 const external='https://cdn.example/media/other/stream.m3u8';
 await f.global.sameframeAdaptive.prepare(7,external,false,()=>{});
 f.video.src=external;
 await assert.rejects(f.global.sameframeAdaptive.ready(7),/connection deadline/);
});
test('buffer readiness checks the current frame, not a later disjoint range',()=>{
 const f=fixture();Object.assign(f.video,{readyState:3,seeking:false,currentTime:5,duration:20,buffered:{length:1,start:()=>8,end:()=>18}});
 assert.equal(f.global.sameframeAdaptive.buffered(7),false);
 f.video.buffered={length:1,start:()=>4,end:()=>8};
 assert.equal(f.global.sameframeAdaptive.buffered(7),true);
 f.video.seeking=true;assert.equal(f.global.sameframeAdaptive.buffered(7),false);
 f.video.seeking=false;f.video.readyState=2;assert.equal(f.global.sameframeAdaptive.buffered(7),false);
});
