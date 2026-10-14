import { defineConfig } from 'vite';
import { leatherTexture } from './plugins/leather-texture.mjs';

export default defineConfig({
	base: './',
	input: ['src/main.ts', 'src/style.scss'],
	plugins: [leatherTexture()],
	build: {
		assetsDir: '',
		emptyOutDir: true,
		manifest: true,
	},
});
