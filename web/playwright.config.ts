import { defineConfig } from '@playwright/test';
export default defineConfig({testDir:'tests',workers:1,use:{baseURL:'http://127.0.0.1:8080',viewport:{width:1440,height:1000},trace:'retain-on-failure'},reporter:'list',webServer:{command:'go run ../cmd/preview',url:'http://127.0.0.1:8080/healthz',reuseExistingServer:!process.env.CI,timeout:60000}});
