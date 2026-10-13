// An owned tag takes precedence over static scheme tags without mutating React
// metadata. Updating hoisted tags directly can make React create duplicates.
export const themeColors={light:'#edf3ff',dark:'#0c1730'} as const;
export function applyTheme(theme:'light'|'dark') {
 document.documentElement.dataset.theme=theme;
 let meta=document.getElementById('site-theme-color');
 if(!meta){meta=document.createElement('meta');meta.id='site-theme-color';meta.setAttribute('name','theme-color');}
 meta.setAttribute('content',themeColors[theme]);
 document.head.prepend(meta);
}
// Executed before body paint; storage access is optional. The frame also places
// our override ahead of metadata tags streamed after the bootstrap script.
export const themeScript = `(function(){var p='system';try{var s=localStorage.getItem('devcourse-theme');if(s==='light'||s==='dark')p=s}catch(e){}var t=p==='system'?(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'):p;document.documentElement.dataset.theme=t;function chrome(){var m=document.getElementById('site-theme-color');if(!m){m=document.createElement('meta');m.id='site-theme-color';m.setAttribute('name','theme-color')}m.setAttribute('content',t==='dark'?'#0c1730':'#edf3ff');document.head.prepend(m)}chrome();requestAnimationFrame(chrome)})()`;
