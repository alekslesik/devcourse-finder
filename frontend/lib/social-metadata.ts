import type {Metadata} from 'next';

// Compose passes SITE_URL at build time and runtime for static and dynamic pages.
export const siteOrigin = new URL(process.env.SITE_URL || 'http://localhost:3000');
export const siteTitle = 'DevCourse — найдите свой путь в разработку';
export const siteDescription = 'Подбор и сравнение обучения Go, Python, Java и JavaScript по опыту, цели и бюджету.';
const preview = {
  url: new URL('/social-preview-v1.png', siteOrigin).href,
  width: 1200,
  height: 630,
  alt: 'DevCourse — учиться тому, что нужно вам. Go, Python, Java и JavaScript.',
  type: 'image/png',
};

export function socialMetadata(title: string, description: string, path: string): Metadata {
  return {
    title, description,
    alternates: {canonical: path},
    openGraph: {
      type: 'website', locale: 'ru_RU', siteName: 'DevCourse',
      title, description, url: new URL(path, siteOrigin).href,
      images: [preview],
    },
    twitter: {
      card: 'summary_large_image', title, description,
      images: [{url: preview.url, alt: preview.alt}],
    },
  };
}
