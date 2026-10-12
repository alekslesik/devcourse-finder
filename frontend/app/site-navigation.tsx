'use client';
import {usePathname} from 'next/navigation';
export default function SiteNavigation() {
  const path=usePathname();
  return <nav className="siteNav" aria-label="Основная навигация">
    <a href="/courses" aria-current={path==='/'||path.startsWith('/courses')?'page':undefined}>Каталог</a>
    <a href="/about" aria-current={path==='/about'?'page':undefined}>О сервисе</a>
  </nav>;
}
