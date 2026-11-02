# Cover site

The server ships with a built-in streaming site ("NOVA"): landing page, catalog,
login form, video player, 404 and legal pages.

- Default behavior: if `fallback.dir` is not set, the embedded site is served.
- To customize it, export the built-in files and edit them:

```bash
kvn-server --export-cover-site /var/lib/kvn/cover
```

Then point the server at the directory:

```yaml
fallback:
  dir: /var/lib/kvn/cover
```

`scripts/install-server.sh` exports the site to `/var/lib/kvn/cover` and sets
`fallback.dir` automatically.
