import type { Metadata } from 'next';
import './globals.css';
import {SiteHeader,SiteFooter} from './site-shell';
import {themeScript} from '../lib/theme';
export const metadata: Metadata = {metadataBase:new URL(process.env.SITE_URL||'http://localhost:3000'),alternates:{canonical:'/'},title:'DevCourse — найдите свой путь в разработку',description:'Подбор и сравнение обучения Go, Python, Java и JavaScript по опыту, цели и бюджету.'};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:themeScript}}/></head><body><SiteHeader/><div id="content" tabIndex={-1}>{children}</div><SiteFooter/></body></html>}