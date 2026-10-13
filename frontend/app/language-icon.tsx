// The transparent atlas is adapted from the maintainer's glass-icon reference.
// View boxes isolate each silhouette; intrinsic widths and optical height
// adjustments balance wide Go lettering against the fine, tall Java outline.
// Java's stepped clip excludes the adjacent Go reflection while retaining its wide saucer.
const art:Record<string,[number,number,number,number]>={
 go:[0,225,700,345],
 java:[705,30,520,660],
 javascript:[50,710,585,445],
 python:[690,700,550,515],
};
export default function LanguageIcon({language}:{language:string}) {
 const bounds=art[language];
 return bounds?<svg className="languageIcon" data-language={language} viewBox={bounds.join(' ')} style={{aspectRatio:`${bounds[2]} / ${bounds[3]}`}} aria-hidden="true" focusable="false"><image href="/languages/glass-icons-v1.webp" width="1254" height="1254" style={language==='java'?{clipPath:'polygon(59% 0,100% 0,100% 100%,56% 100%,56% 44%,59% 44%)'}:undefined}/></svg>:null;
}
