 'use client';
export default function ErrorPage({reset}:{reset:()=>void}){return <main className="document"><h1>Каталог временно недоступен</h1><p role="alert">Не удалось получить данные. Повторите попытку.</p><button onClick={reset}>Повторить</button> <a href="/courses">Вернуться в каталог</a></main>}
