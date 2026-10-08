type Props = {
  kind: 'error' | 'empty' | 'filtered';
  message?: string;
  onRetry: () => void;
  onReset?: () => void;
};

export default function ResultState({kind, message, onRetry, onReset}: Props) {
  const title = kind === 'error' ? 'Не удалось загрузить каталог'
    : kind === 'empty' ? 'Программы пока не опубликованы' : 'Подходящая программа ещё не нашлась';
  const description = kind === 'error' ? message
    : kind === 'empty' ? 'Каталог ещё наполняется. Проверьте обновления немного позже.'
    : 'Попробуйте увеличить бюджет или убрать часть условий. Ваши фильтры сохранены.';
  return <div className={`empty resultState ${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
    <div className="stateIcon" aria-hidden="true"><svg viewBox="0 0 32 32" focusable="false"><rect x="5" y="5" width="22" height="22" rx="6" fill="none" stroke="currentColor" strokeWidth="2"/>{kind==='error'?<path d="M16 10v7m0 4v1" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"/>:kind==='empty'?<path d="M11 12h10m-10 5h7m-7 5h4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>:<><circle cx="14" cy="14" r="5" fill="none" stroke="currentColor" strokeWidth="2"/><path d="m18 18 5 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/></>}</svg></div>
    <h3>{title}</h3><p>{description}</p>
    <div className="stateActions">
      {kind !== 'filtered' && <button className="primary" onClick={onRetry}>{kind === 'error' ? 'Повторить' : 'Обновить каталог'}</button>}
      {kind !== 'empty' && onReset && <button className={kind === 'filtered' ? 'primary' : ''} onClick={onReset}>{kind === 'filtered' ? 'Показать все программы' : 'Сбросить фильтры'}</button>}
    </div>
  </div>;
}
