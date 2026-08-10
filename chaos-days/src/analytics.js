export const GOATCOUNTER_ENDPOINT = 'https://chriskujawa.goatcounter.com/count';
export const PRODUCTION_HOST = 'camunda.github.io';

function isBot() {
  const {document, navigator, window} = globalThis;

  if (window.callPhantom || window._phantom || window.phantom) {
    return 150;
  }

  if (window.__nightmare) {
    return 151;
  }

  if (
    document.__selenium_unwrapped ||
    document.__webdriver_evaluate ||
    document.__driver_evaluate
  ) {
    return 152;
  }

  if (navigator.webdriver) {
    return 153;
  }

  return 0;
}

function shouldSkipAnalytics() {
  if (window.location.hostname !== PRODUCTION_HOST) {
    return true;
  }

  if ('visibilityState' in document && document.visibilityState === 'prerender') {
    return true;
  }

  if (window.location !== window.parent.location) {
    return true;
  }

  return localStorage.getItem('skipgc') === 't';
}

function createTrackingUrl({path, title}) {
  const parameters = new URLSearchParams({
    p: path,
    t: title,
    r: document.referrer,
    s: String(window.screen.width),
    q: window.location.search,
    rnd: Math.random().toString(36).slice(2, 7),
  });
  const bot = isBot();

  if (bot !== 0) {
    parameters.set('b', String(bot));
  }

  return `${GOATCOUNTER_ENDPOINT}?${parameters.toString()}`;
}

export function trackPageView({path, title = document.title}) {
  if (shouldSkipAnalytics()) {
    return;
  }

  const url = createTrackingUrl({path, title});

  if (navigator.sendBeacon?.(url)) {
    return;
  }

  const image = document.createElement('img');
  image.src = url;
  image.style.position = 'absolute';
  image.style.bottom = '0';
  image.style.width = '1px';
  image.style.height = '1px';
  image.loading = 'eager';
  image.alt = '';
  image.setAttribute('aria-hidden', 'true');

  image.addEventListener(
    'load',
    () => {
      image.remove();
    },
    {once: true},
  );

  document.body.appendChild(image);
}
