import type { Metadata } from 'next';
import './globals.css';
export const metadata: Metadata = {title:'DevCourse — найдите свой путь в разработку',description:'Подбор и сравнение обучения Go, Python, Java и JavaScript по опыту, цели и бюджету.'};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru"><body>{children}</body></html>}