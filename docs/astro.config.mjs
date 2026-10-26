// @ts-check
import { defineConfig, fontProviders } from 'astro/config';
import starlight from '@astrojs/starlight';

import mermaid from 'astro-mermaid';
import catppuccin from "@catppuccin/starlight";

// https://astro.build/config
export default defineConfig({
	experimental: {
		fonts: [
			{
				provider: fontProviders.google(),
				name: "Victor Mono",
				cssVariable: "--font-victor-mono",
			},
			{
				provider: fontProviders.google(),
				name: "JetBrains Mono",
				cssVariable: "--font-jetbrains-mono",
			},
		],
	},
	integrations: [
		mermaid({
			theme: 'forest',
			autoTheme: true
		}),
		starlight({
			title: '{mtx}',
			social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/vic/mtx' }
      ],
			sidebar: [
				{
					label: 'Start',
					items: [{ label: 'Start', slug: 'start' }],
				},
				{
					label: 'Use',
					items: [
						{ label: 'Moving around', slug: 'moving' },
						{ label: 'Writing', slug: 'writing' },
						{ label: 'Rooms and people', slug: 'rooms' },
						{ label: 'Threads and pins', slug: 'threads' },
						{ label: 'Find things', slug: 'find' },
						{ label: 'Media', slug: 'media' },
						{ label: 'Looks', slug: 'looks' },
						{ label: 'Security', slug: 'security' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Keys', slug: 'keys' },
					],
				},
			],
			components: {
				Head: './src/components/Head.astro',
				Sidebar: './src/components/Sidebar.astro',
				Footer: './src/components/Footer.astro',
				SocialIcons: './src/components/SocialIcons.astro',
				PageSidebar: './src/components/PageSidebar.astro',
				Hero: './src/components/Hero.astro',
			},
			plugins: [
				catppuccin({
					dark: { flavor: "macchiato", accent: "mauve" },
					light: { flavor: "latte", accent: "mauve" },
				}),
			],
			editLink: {
				baseUrl: 'https://github.com/vic/mtx/edit/main/docs/',
			},
			customCss: [
				'./src/styles/custom.css'
			],
		}),
	],
});
