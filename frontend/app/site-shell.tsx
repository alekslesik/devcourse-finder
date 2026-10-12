import Brand from './brand';
import ThemeControl from './theme-control';
import SiteNavigation from './site-navigation';
export function SiteHeader() {
  return <><a className="skipLink" href="#content">Перейти к содержанию</a><header>
    <Brand/>
    <SiteNavigation/>
    <ThemeControl/>
  </header></>;
}
export function SiteFooter() {
  return <footer><Brand/><span>Учиться — ваш выбор. Найти — наша задача.</span><a href="/about">Как устроен каталог</a><span className="siteVersion" aria-label={'Версия сайта v'+process.env.NEXT_PUBLIC_APP_VERSION}>v{process.env.NEXT_PUBLIC_APP_VERSION}</span></footer>;
}
