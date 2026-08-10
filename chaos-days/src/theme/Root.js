import React, {useEffect} from 'react';
import {useLocation} from '@docusaurus/router';

function trackPageView(path) {
  if (typeof window.goatcounter?.count !== 'function') {
    return false;
  }

  window.goatcounter.count({
    path,
    title: document.title,
  });

  return true;
}

export default function Root({children}) {
  const location = useLocation();

  useEffect(() => {
    const path = `${location.pathname}${location.search}`;

    if (trackPageView(path)) {
      return undefined;
    }

    const script = document.querySelector('script[data-goatcounter]');

    if (script === null) {
      return undefined;
    }

    const handleLoad = () => {
      trackPageView(path);
    };

    script.addEventListener('load', handleLoad, {once: true});

    return () => {
      script.removeEventListener('load', handleLoad);
    };
  }, [location.pathname, location.search]);

  return <>{children}</>;
}
