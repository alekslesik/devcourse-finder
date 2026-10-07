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
  return <label className="themeControl">Тема<select aria-label="Тема" value={preference} onChange={e => {
    const next = e.target.value as Preference; setPreference(next);
    try {localStorage.setItem('devcourse-theme', next);} catch {}
  }}><option value="system">Системная</option><option value="light">Светлая</option><option value="dark">Тёмная</option></select></label>;
}
