import type {MetadataRoute} from 'next';
import {siteOrigin} from '../lib/social-metadata';

export default function robots():MetadataRoute.Robots{
 // Crawlers must be able to read noindex on catalog and comparison pages.
 return {rules:{userAgent:'*',allow:'/'},sitemap:new URL('/sitemap.xml',siteOrigin).href};
}
