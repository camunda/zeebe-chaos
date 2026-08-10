# Website

This website is built using [Docusaurus 3](https://docusaurus.io/), a modern static website generator.

### Installation

```shell
make install
```

### Local Development

```shell
make serve
```

This command starts a local development server and opens up a browser window. Most changes are reflected live without having to restart the server.

### Build

```shell
make build
```

This command generates static content into the `build` directory and can be served using any static contents hosting service.

### Usage analytics

The production GitHub Pages site uses GoatCounter for privacy-friendly page view analytics. The official GoatCounter script is hosted from the site's static assets so browsers do not have to load `gc.zgo.at/count.js`. Docusaurus route changes are tracked explicitly so navigating between posts in the single-page app records page views.
