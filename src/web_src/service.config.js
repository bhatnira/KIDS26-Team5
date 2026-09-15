// different query service config
export const serviceConfig = {
  'dev': {
    url: 'http://localhost:8086/api/v1',
  },
  // Relative path: the production frontend is embedded in and served
  // same-origin as the Go backend, so API calls resolve against whatever
  // scheme/host/port served the page (localhost:8086, a reverse proxy, a real
  // domain, …). Do NOT hardcode an absolute URL here.
  'production': {
    url: '/api/v1',
  },
}
