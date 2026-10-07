// Executed before body paint; storage access is optional.
export const themeScript = `(function(){var p='system';try{var s=localStorage.getItem('devcourse-theme');if(s==='light'||s==='dark')p=s}catch(e){}document.documentElement.dataset.theme=p==='system'?(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'):p})()`;
