import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./tests',workers:1,use:{baseURL:'http://127.0.0.1:3091',trace:'retain-on-failure',launchOptions:process.env.CHROMIUM_PATH?{executablePath:process.env.CHROMIUM_PATH}:{}},webServer:[
 {command:'node scripts/fixture-api.mjs',url:'http://127.0.0.1:8091/api/v1/courses',reuseExistingServer:false},
 {command:'node node_modules/next/dist/bin/next start -p 3091 -H 127.0.0.1',url:'http://127.0.0.1:3091/about',reuseExistingServer:false,env:{API_URL:'http://127.0.0.1:8091',SITE_URL:'http://127.0.0.1:3091'}}
]});
