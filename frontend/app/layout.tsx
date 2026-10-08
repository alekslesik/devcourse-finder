import type { Metadata } from 'next';
import './globals.css';
import {socialMetadata,siteOrigin,siteTitle,siteDescription} from '../lib/social-metadata';
import {SiteHeader,SiteFooter} from './site-shell';
import {themeScript} from '../lib/theme';
export const metadata: Metadata = {metadataBase:siteOrigin,...socialMetadata(siteTitle,siteDescription,'/')};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:themeScript}}/></head><body><SiteHeader/><div id="content" tabIndex={-1}>{children}</div><SiteFooter/></body></html>}