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
    <div className="stateIcon" aria-hidden="true">{kind === 'error' ? '!' : kind === 'empty' ? '◇' : '⌕'}</div>
    <h3>{title}</h3><p>{description}</p>
    <div className="stateActions">
      {kind !== 'filtered' && <button className="primary" onClick={onRetry}>{kind === 'error' ? 'Повторить' : 'Обновить каталог'}</button>}
      {kind !== 'empty' && onReset && <button className={kind === 'filtered' ? 'primary' : ''} onClick={onReset}>{kind === 'filtered' ? 'Показать все программы' : 'Сбросить фильтры'}</button>}
    </div>
  </div>;
}
