/* SameFrame bridge for the pinned media_kit 1.2.6 WebPlayer. */
(() => {
  'use strict';
  const states = new Map();
  const subtitles = new Map();
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
    const subtitle = subtitles.get(id);
    if (subtitle) {
      subtitle.track.remove();
      URL.revokeObjectURL(subtitle.url);
      subtitles.delete(id);
    }
    const state = states.get(id);
    if (!state) return;
    state.active = false;
    state.reject?.(new Error('播放源已切换'));
    try { if (state.engine) await state.engine.destroy(); } catch (_) {}
    delete state.video.src;
    states.delete(id);
  }
  async function prepare(id, url, live, onError, startPosition = 0) {
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
          const config={streaming:{bufferingGoal:15,rebufferingGoal:2}};
          const target=new URL(url,document.baseURI);
          if(kind==='hls'&&target.origin===new URL(document.baseURI).origin&&target.pathname.startsWith('/media/')) {
            // Private providers can prepare/transcode the first fragment before
            // responding. The default 10s connection deadline would cancel
            // every cold start before the provider's bounded 90s work completes.
            config.manifest={retryParameters:{timeout:120000,connectionTimeout:95000,stallTimeout:95000}};
            config.streaming.retryParameters={timeout:120000,connectionTimeout:95000,stallTimeout:45000};
          }
          engine.configure(config);
          // Set the start before Shaka initializes its playhead; an early HTML seek can be overwritten.
          const start = Number.isFinite(startPosition) && startPosition > 0 ? startPosition : undefined;
          await engine.load(url,start,kind==='dash'?'application/dash+xml':'application/x-mpegurl');
          if(state.active)state.resolve();
        }
      } catch(error) { if(state.active) fail(error?.message??'媒体加载失败'); }
    };
    Object.defineProperty(video,'src',{configurable:true,enumerable:true,
      get(){return nativeSrc.get.call(video)},
      set(value){if(state.active&&value&&canonical(value)===canonical(url)){void load()}else{nativeSrc.set.call(video,value)}}
    });
  }
  const buffered = id => {
    const video=globalThis['$com.alexmercerind.media_kit.instances']?.[String(id)];
    if(!video||video.seeking||video.readyState<3)return false;
    const needed=Math.min(.75,Math.max(0,video.duration-video.currentTime));
    for(let i=0;i<video.buffered.length;i++) {
      if(video.buffered.start(i)<=video.currentTime+.05&&video.buffered.end(i)>=video.currentTime+needed)return true;
    }
    return false;
  };
  const element = id => globalThis['$com.alexmercerind.media_kit.instances']?.[String(id)];
  function info(id) {
    const video=element(id), engine=states.get(id)?.engine;
    const tracks=engine?.getVariantTracks?.()??[];
    const seen=new Set();
    const qualities=tracks.filter(t=>t.height&&seen.size<20&&!seen.has(t.height)&&seen.add(t.height))
      .sort((a,b)=>b.height-a.height).map(t=>({id:t.id,label:t.height+'p'}));
    return JSON.stringify({pip:!!(document.pictureInPictureEnabled&&video?.requestPictureInPicture),
      screenshot:!!video?.videoWidth,qualities,
      audio:[...new Set(tracks.map(t=>t.language).filter(x=>x&&x!=='und'))]});
  }
  async function action(id,name,value) {
    const video=element(id), engine=states.get(id)?.engine;
    if(!video)throw Error('播放器尚未就绪');
    if(name==='pip') {
      if(document.pictureInPictureElement===video)await document.exitPictureInPicture();
      else await video.requestPictureInPicture();
    } else if(name==='quality') {
      if(!engine?.getVariantTracks)throw Error('当前片源没有可切换的画质');
      if(value==='auto')engine.configure({abr:{enabled:true}});
      else {
        const track=engine.getVariantTracks().find(t=>String(t.id)===value);
        if(!track)throw Error('该画质已不可用，请重新选择');
        engine.configure({abr:{enabled:false}});engine.selectVariantTrack(track,true);
      }
    } else if(name==='audio') {
      if(!engine?.selectAudioLanguage)throw Error('当前片源没有可切换的音轨');
      engine.selectAudioLanguage(value);
    } else if(name==='screenshot') {
      if(!video.videoWidth)throw Error('画面尚未加载');
      const canvas=document.createElement('canvas');canvas.width=video.videoWidth;canvas.height=video.videoHeight;
      canvas.getContext('2d').drawImage(video,0,0);
      const blob=await new Promise((resolve,reject)=>canvas.toBlob(b=>b?resolve(b):reject(Error('截图失败')),'image/png'));
      const url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download='sameframe-'+Date.now()+'.png';a.click();
      setTimeout(()=>URL.revokeObjectURL(url),5000);
    } else if(name==='subtitle') {
      const old=subtitles.get(id);
      if(old){old.track.remove();URL.revokeObjectURL(old.url);subtitles.delete(id);}
      if(value) {
        let content=value.replace(/^\uFEFF/,'').replace(/\r\n/g,'\n');
        if(!content.startsWith('WEBVTT'))content='WEBVTT\n\n'+content.replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g,'$1.$2');
        const url=URL.createObjectURL(new Blob([content],{type:'text/vtt'})),track=document.createElement('track');
        track.kind='subtitles';track.label='本地字幕';track.src=url;track.default=true;
        video.appendChild(track);track.track.mode='showing';subtitles.set(id,{track,url});
      }
    } else throw Error('不支持的播放操作');
  }
  globalThis.sameframeAdaptive={prepare,ready:id=>states.get(id)?.promise??Promise.resolve(),buffered,dispose,info,action};
})();
