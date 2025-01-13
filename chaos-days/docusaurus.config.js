const lightCodeTheme = require('prism-react-renderer/themes/github');
const darkCodeTheme = require('prism-react-renderer/themes/dracula');

// With JSDoc @type annotations, IDEs can provide config autocompletion
/** @type {import('@docusaurus/types').DocusaurusConfig} */
(module.exports = {
  title: 'Zeebe Chaos',
  tagline: 'Chaos Day Summaries',
  url: 'https://camunda.github.io',
  baseUrl: '/zeebe-chaos/',
  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',
  favicon: 'img/zeebe-logo.png',
  organizationName: 'camunda', // Usually your GitHub org/user name.
  projectName: 'zeebe-chaos', // Usually your repo name.

  plugins: [
    'plugin-image-zoom'
  ],

  presets: [
    [
      '@docusaurus/preset-classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: false,
        blog: {
          showReadingTime: true,
          // Please change this to your repo.
          editUrl:
            'https://github.com/camunda/zeebe-chaos/blob/master/chaos-days/',
          routeBasePath: '/',
          blogSidebarTitle: 'All posts',
          blogSidebarCount: 'ALL',
        },
        theme: {
          customCss: require.resolve('./src/css/custom.css'),
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      navbar: {
        title: 'Zeebe Chaos',
        logo: {
          alt: 'Zeebe',
          src: 'img/zeebe-logo.png',
        },
        items: [

          {to: '/', label: 'Chaos Summaries', position: 'left'},
          {
            href: 'https://github.com/camunda/zeebe-chaos',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Community',
            items: [
              {
                label: 'Stack Overflow',
                href: 'https://stackoverflow.com/questions/tagged/zeebe',
              },
              {
                label: 'Forum',
                href: 'https://forum.camunda.io/'
              },
              {
                label: 'Twitter',
                href: 'https://twitter.com/Camunda',
              },
            ],
          },
          {
            title: 'More',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/camunda/zeebe-chaos/',
              },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Zeebe Chaos. Built with Docusaurus.`,
      },
      prism: {
        theme: lightCodeTheme,
        darkTheme: darkCodeTheme,
      },
      zoomSelector: '.markdown img',
    }),
});
