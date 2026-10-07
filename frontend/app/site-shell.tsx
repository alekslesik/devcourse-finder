import ThemeControl from './theme-control';
export function SiteHeader() {
  return <><a className="skipLink" href="#content">Перейти к содержанию</a><header>
    <a className="brand" href="/"><span className="brandIcon">&lt;/&gt;</span>devcourse<span className="brandDot">.</span></a>
    <nav className="siteNav" aria-label="Основная навигация"><a href="/courses">Каталог</a><a href="/about">О сервисе</a></nav>
    <ThemeControl/>
  </header></>;
}
export function SiteFooter() {
  return <footer><a className="brand" href="/">devcourse.</a><span>Учиться — ваш выбор. Найти — наша задача.</span><a href="/about">Как устроен каталог</a></footer>;
}
