'use client';
import {useEffect, useState} from 'react';
type Preference = 'system' | 'light' | 'dark';
const valid = (value: string | null): value is Preference => ['system', 'light', 'dark'].includes(value || '');
export default function ThemeControl() {
  const [preference, setPreference] = useState<Preference>('system');
  const [ready, setReady] = useState(false);
  useEffect(() => {
    try {const saved = localStorage.getItem('devcourse-theme'); if (valid(saved)) setPreference(saved);} catch {}
    setReady(true);
  }, []);
  useEffect(() => {
    if (!ready) return;
    const media = matchMedia('(prefers-color-scheme: dark)');
    const apply = () => {document.documentElement.dataset.theme = preference === 'system' ? (media.matches ? 'dark' : 'light') : preference;};
    apply(); media.addEventListener('change', apply);
    return () => media.removeEventListener('change', apply);
  }, [preference, ready]);
  const choose = (next: Preference) => {
    setPreference(next);
    try {localStorage.setItem('devcourse-theme', next);} catch {}
  };
  return <div className="themeControl" role="radiogroup" aria-label="Тема">
    {(['light', 'system', 'dark'] as const).map(value => {
      const label = value === 'light' ? 'Светлая тема' : value === 'dark' ? 'Тёмная тема' : 'Системная тема';
      return <label key={value} className={`themeChoice ${value}`} title={label}>
        <input type="radio" name="theme-preference" aria-label={label} value={value} checked={preference === value} onChange={() => choose(value)}/>
        <span aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" focusable="false">
          {value === 'light' ? <><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/></>
            : value === 'dark' ? <path d="M20 14a8.5 8.5 0 0 1-10-10 8.5 8.5 0 1 0 10 10Z" fill="currentColor" stroke="none"/>
            : <circle cx="12" cy="12" r="9" fill="currentColor" stroke="none"/>}
        </svg></span>
      </label>;
    })}
  </div>;
}
