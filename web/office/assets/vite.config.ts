import { defineConfig } from 'vite';

export default defineConfig({
	base: './',
	input: ['src/main.ts', 'src/style.scss'],
	build: {
		assetsDir: '',
		emptyOutDir: true,
		manifest: true,
	},
});
