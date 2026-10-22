'use client';
import CatalogReturn from './catalog-return';
import {usePathname} from 'next/navigation';
export default function SiteNavigation() {
  const path=usePathname();
  return <nav className="siteNav" aria-label="Основная навигация">
    {path.startsWith('/courses/')?<CatalogReturn label="Каталог" current/>:<a href="/courses" aria-current={path==='/'||path==='/courses'?'page':undefined}>Каталог</a>}
    <a href="/about" aria-current={path==='/about'?'page':undefined}>О сервисе</a>
  </nav>;
}
