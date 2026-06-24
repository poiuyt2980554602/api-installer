# Pixel Image Playground

This package builds a self-hosted image playground from `CookSleep/gpt_image_playground` and adds Pixel API key import compatibility.

Pixel/Sub2API can open:

```text
/#/import/pixel?payload=<base64url-json>
```

The compatibility patch converts that payload to the upstream playground URL settings format, imports the API base URL and API key into the browser profile store, then removes the sensitive import payload from the address bar.
