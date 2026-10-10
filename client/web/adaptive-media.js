/* SameFrame bridge for the pinned media_kit 1.2.6 WebPlayer. */
(() => {
  'use strict';
  const states = new Map();
  const nativeSrc = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'src');
  const canonical = value => new URL(value, document.baseURI).href;
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  async function videoFor(id) {
    const deadline = Date.now() + 15000;
    while (Date.now() < deadline) {
      const video = globalThis['$com.alexmercerind.media_kit.instances']?.[String(id)];
      // media_kit initializes its bundled HLS script asynchronously.
      if (video && globalThis.Hls) return video;
      await delay(10);
    }
    throw new Error('播放器初始化超时');
  }
  async function dispose(id) {
    const state = states.get(id);
    if (!state) return;
    state.active = false;
    state.reject?.(new Error('播放源已切换'));
    try { if (state.engine) await state.engine.destroy(); } catch (_) {}
    delete state.video.src;
    states.delete(id);
  }
  async function prepare(id, url, live, onError) {
    const video = await videoFor(id);
    await dispose(id);
    // One adapter owns HLS lifecycles; the old media_kit helper has no exposed
    // destroy handle. All non-adaptive URLs still use the native video element.
    globalThis.Hls.isSupported = () => false;
    const kind = /\.mpd(?:[?#]|$)/i.test(url) ? 'dash' : /\.m3u8(?:[?#]|$)|\/stream\.hls/i.test(url) ? 'hls' : /\.flv(?:[?#]|$)/i.test(url) ? 'flv' : '';
    if (!kind) return;
    const state = {video, active:true, engine:null, promise:null, resolve:null, reject:null};
    state.promise = new Promise((resolve,reject) => {state.resolve=resolve;state.reject=reject});
    state.promise.catch(() => {});
    states.set(id,state);
    const fail = message => {
      if (!state.active) return;
      onError(message);
      state.reject(new Error(message));
    };
    const load = async () => {
      try {
        if (kind === 'flv') {
          if (!globalThis.mpegts?.isSupported()) throw new Error('浏览器不支持该直播格式');
          const engine = globalThis.mpegts.createPlayer({type:'flv',isLive:live,url}, {enableWorker:false,lazyLoad:!live});
          state.engine=engine;
          engine.on(globalThis.mpegts.Events.ERROR, (_type,detail) => fail('直播播放失败：'+String(detail)));
          video.addEventListener('loadedmetadata',()=>{if(state.active)state.resolve()},{once:true});
          engine.attachMediaElement(video);
          engine.load();
        } else {
          globalThis.shaka.polyfill.installAll();
          if (!globalThis.shaka.Player.isBrowserSupported()) throw new Error('浏览器不支持该播放格式');
          const engine=new globalThis.shaka.Player();
          state.engine=engine;
          engine.addEventListener('error',event=>{
            const e=event.detail;
            const status=Array.isArray(e?.data)?e.data.find(v=>typeof v==='number'&&v>=400&&v<=599):null;
            fail(status?'HTTP '+status+'：媒体请求失败':'媒体播放失败（'+String(e?.code??'unknown')+'）');
          });
          await engine.attach(video);
          engine.configure({streaming:{bufferingGoal:15,rebufferingGoal:2}});
          await engine.load(url,undefined,kind==='dash'?'application/dash+xml':'application/x-mpegurl');
          if(state.active)state.resolve();
        }
      } catch(error) { if(state.active) fail(error?.message??'媒体加载失败'); }
    };
    Object.defineProperty(video,'src',{configurable:true,enumerable:true,
      get(){return nativeSrc.get.call(video)},
      set(value){if(state.active&&value&&canonical(value)===canonical(url)){void load()}else{nativeSrc.set.call(video,value)}}
    });
  }
  globalThis.sameframeAdaptive={prepare,ready:id=>states.get(id)?.promise??Promise.resolve(),dispose};
})();
