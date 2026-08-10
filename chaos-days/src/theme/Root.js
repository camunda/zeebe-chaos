import React, {useEffect} from 'react';
import {useLocation} from '@docusaurus/router';
import {trackPageView} from '../analytics';

export default function Root({children}) {
  const location = useLocation();

  useEffect(() => {
    trackPageView({path: `${location.pathname}${location.search}`});
  }, [location.pathname, location.search]);

  return <>{children}</>;
}
