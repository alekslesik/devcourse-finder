import Brand from './brand';
import ThemeControl from './theme-control';
export function SiteHeader() {
  return <><a className="skipLink" href="#content">Перейти к содержанию</a><header>
    <Brand/>
    <nav className="siteNav" aria-label="Основная навигация"><a href="/courses">Каталог</a><a href="/about">О сервисе</a></nav>
    <ThemeControl/>
  </header></>;
}
export function SiteFooter() {
  return <footer><Brand/><span>Учиться — ваш выбор. Найти — наша задача.</span><a href="/about">Как устроен каталог</a></footer>;
}
