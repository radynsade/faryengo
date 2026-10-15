import { defineConfig } from 'vite';
import { textures } from './plugins/textures.mjs';

export default defineConfig({
	base: './',
	input: ['src/main.ts', 'src/style.scss'],
	plugins: [textures()],
	build: {
		assetsDir: '',
		emptyOutDir: true,
		manifest: true,
	},
});
