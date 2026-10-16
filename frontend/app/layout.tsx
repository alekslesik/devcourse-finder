import type { Metadata,Viewport } from 'next';
import './globals.css';
import './liquid-glass.css';
import './course-passport.css';
import {socialMetadata,siteOrigin,siteTitle,siteDescription} from '../lib/social-metadata';
import {SiteHeader,SiteFooter} from './site-shell';
import {themeScript} from '../lib/theme';
export const viewport:Viewport = {width:'device-width',initialScale:1,viewportFit:'cover',themeColor:[{media:'(prefers-color-scheme: light)',color:'#edf3ff'},{media:'(prefers-color-scheme: dark)',color:'#0c1730'}],colorScheme:'light dark'};
export const metadata: Metadata = {metadataBase:siteOrigin,icons:{icon:[{url:'/favicon.svg',type:'image/svg+xml'},{url:'/favicon-32.png',type:'image/png',sizes:'32x32'}],apple:'/apple-touch-icon.png'},...socialMetadata(siteTitle,siteDescription,'/')};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:themeScript}}/></head><body><SiteHeader/><div id="content" tabIndex={-1}>{children}</div><SiteFooter/></body></html>}
