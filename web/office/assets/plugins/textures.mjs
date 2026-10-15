// A Vite plugin that renders the interface's surface textures. Stylesheets
// import each one as `@generated/<name>.png`; the plugin renders them into
// Vite's cache when Vite starts, and the build hashes and emits them like any
// other asset. Textures are rasters because equivalent SVG filters make
// browsers lag while painting the page.
//
// Noise is shaped in the frequency domain, which makes every tile repeat
// seamlessly. Each texture sets its density: device pixels per CSS pixel. A
// density of 2 keeps fine grain sharp on high-density screens.
import { mkdirSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { crc32, deflateSync } from 'node:zlib';

const directory = fileURLToPath(
	new URL('../node_modules/.vite/generated/', import.meta.url),
);

/** @returns {import('vite').Plugin} */
export function textures() {
	return {
		name: 'office:textures',
		config() {
			mkdirSync(directory, { recursive: true });
			const alias = {};

			for (const [name, render] of Object.entries(recipes)) {
				const file = `${directory}${name}.png`;
				writeFileSync(file, render());
				alias[`@generated/${name}.png`] = file;
			}

			return { resolve: { alias } };
		},
	};
}

// Saffiano leather behind the sign-in showcase, shown at 256px. Grayscale with
// flat areas at 50% gray, so a soft-light blend leaves the colors beneath
// unchanged and adds only the relief.
function leather() {
	const size = 512;
	const density = 2;
	const next = random(7);
	// Diagonal hatch cuts: frequencies in cycles per CSS pixel along and across
	// the cut, and the direction of each cut set in degrees.
	const hatch = { along: 0.07, across: 0.42, angles: [45, -45] };
	// Fine pebble grain under the hatch, and its share of the relief.
	const grain = { frequency: 0.22, weight: 0.6 };
	// Relief depth: higher values make the leather more pronounced.
	const depth = 1;
	// Gray levels are rounded to this step. The soft-light blend hides steps
	// this small, and fewer levels compress the PNG far better.
	const step = 6;

	const layers = [];
	for (const degrees of hatch.angles) {
		const noise = shapedNoise(next, size, (fx, fy) =>
			oriented(fx, fy, density, degrees, hatch.along, hatch.across),
		);
		layers.push({ height: ridges(noise, 2.2, 0.75), weight: 1 });
	}
	const noise = shapedNoise(next, size, (fx, fy) =>
		round(fx, fy, density, grain.frequency),
	);
	layers.push({ height: ridges(noise, 1.4, 0.3), weight: grain.weight });

	const shade = relief(stack(layers), size, density, depth);
	const pixels = Buffer.alloc(size * size);
	for (let i = 0; i < pixels.length; i++) {
		pixels[i] = quantize(shade[i] * 255, step);
	}

	return png(size, 1, pixels);
}

// Paper under the whole interface, shown at 256px. Transparent: shadows are
// black and highlights white, each at a low alpha, so it reads on any light
// background with ordinary blending.
function paper() {
	const size = 512;
	const density = 2;
	const next = random(11);
	// Fine tooth of the sheet: frequency in cycles per CSS pixel and relief.
	const grain = { frequency: 0.6, weight: 0.5 };
	// Pressed fibers: short thin strands in several directions. Angles stay off
	// the axes, where strands would line up with the tile into endless lines.
	const fibers = {
		along: 0.06,
		across: 0.5,
		angles: [17, 49, 79, 107, 139, 167],
		weight: 1,
	};
	// Relief depth and the strongest alpha it may reach.
	const depth = 0.6;
	const strength = 0.07;
	// Alpha levels are rounded to this step out of 255.
	const step = 3;

	const tooth = shapedNoise(next, size, (fx, fy) =>
		round(fx, fy, density, grain.frequency),
	);
	const strands = shapedNoise(next, size, (fx, fy) =>
		Math.max(
			...fibers.angles.map((degrees) =>
				oriented(fx, fy, density, degrees, fibers.along, fibers.across),
			),
		),
	);

	const shade = relief(
		stack([
			{ height: ridges(tooth, 1.4, 0.3), weight: grain.weight },
			{ height: ridges(strands, 2, 0.6), weight: fibers.weight },
		]),
		size,
		density,
		depth,
	);

	// Gray and alpha pairs: light above the neutral half, shadow below it.
	const pixels = Buffer.alloc(size * size * 2);
	for (let i = 0; i < size * size; i++) {
		const tone = (shade[i] - 0.5) * 2 * strength;
		pixels[i * 2] = tone > 0 ? 255 : 0;
		pixels[i * 2 + 1] = quantize(Math.abs(tone) * 255, step);
	}

	return png(size, 2, pixels);
}

const recipes = { leather, paper };

function random(state) {
	return () => {
		state = (state + 0x6d2b79f5) | 0;
		let t = Math.imul(state ^ (state >>> 15), 1 | state);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

function quantize(value, step) {
	return Math.min(255, Math.round(value / step) * step);
}

// Isotropic spectrum around a frequency in cycles per CSS pixel.
function round(fx, fy, density, frequency) {
	return Math.exp(-(fx * fx + fy * fy) / (frequency / density) ** 2);
}

// Spectrum of streaks along a direction in degrees, with frequencies in
// cycles per CSS pixel along and across the streaks.
function oriented(fx, fy, density, degrees, along, across) {
	const angle = (degrees * Math.PI) / 180;
	const u = fx * Math.cos(angle) + fy * Math.sin(angle);
	const v = -fx * Math.sin(angle) + fy * Math.cos(angle);
	return Math.exp(
		-((u / (along / density)) ** 2) - (v / (across / density)) ** 2,
	);
}

// In-place radix-2 FFT over one row or column of a size×size grid.
function fft(re, im, size, offset, stride, inverse) {
	for (let i = 1, j = 0; i < size; i++) {
		let bit = size >> 1;
		for (; j & bit; bit >>= 1) j ^= bit;
		j ^= bit;
		if (i < j) {
			const a = offset + i * stride;
			const b = offset + j * stride;
			[re[a], re[b]] = [re[b], re[a]];
			[im[a], im[b]] = [im[b], im[a]];
		}
	}

	for (let length = 2; length <= size; length <<= 1) {
		const angle = ((inverse ? 2 : -2) * Math.PI) / length;
		for (let start = 0; start < size; start += length) {
			for (let k = 0; k < length / 2; k++) {
				const wr = Math.cos(angle * k);
				const wi = Math.sin(angle * k);
				const a = offset + (start + k) * stride;
				const b = offset + (start + k + length / 2) * stride;
				const tr = re[b] * wr - im[b] * wi;
				const ti = re[b] * wi + im[b] * wr;
				re[b] = re[a] - tr;
				im[b] = im[a] - ti;
				re[a] += tr;
				im[a] += ti;
			}
		}
	}
}

function fft2(re, im, size, inverse) {
	for (let y = 0; y < size; y++) fft(re, im, size, y * size, 1, inverse);
	for (let x = 0; x < size; x++) fft(re, im, size, x, size, inverse);
}

// Seamless noise whose spectrum follows filter(fx, fy), normalized to zero
// mean and unit deviation. Frequencies are in cycles per device pixel.
function shapedNoise(next, size, filter) {
	const re = new Float64Array(size * size);
	const im = new Float64Array(size * size);
	for (let i = 0; i < re.length; i++) re[i] = next() - 0.5;

	fft2(re, im, size, false);
	for (let y = 0; y < size; y++) {
		const fy = (y < size / 2 ? y : y - size) / size;
		for (let x = 0; x < size; x++) {
			const fx = (x < size / 2 ? x : x - size) / size;
			const gain = filter(fx, fy);
			re[y * size + x] *= gain;
			im[y * size + x] *= gain;
		}
	}
	fft2(re, im, size, true);

	let sum = 0;
	let squares = 0;
	for (const value of re) {
		sum += value;
		squares += value * value;
	}
	const mean = sum / re.length;
	const deviation = Math.sqrt(squares / re.length - mean * mean);

	return re.map((value) => (value - mean) / deviation);
}

// Turns noise into sparse raised ridges, like an SVG turbulence alpha ramp.
function ridges(noise, gain, cut) {
	return noise.map((z) =>
		Math.min(1, Math.max(0, (0.5 + z * 0.18) * gain - cut)),
	);
}

// Layers stack like translucent paint: each covers what is left uncovered.
function stack(layers) {
	const height = new Float64Array(layers[0].height.length);
	for (let i = 0; i < height.length; i++) {
		let uncovered = 1;
		for (const layer of layers) uncovered *= 1 - layer.height[i] * layer.weight;
		height[i] = 1 - uncovered;
	}
	return height;
}

// Diffuse lighting from the upper left with a Sobel normal, wrapping at the
// edges. A flat surface reflects exactly half of the light.
function relief(height, size, density, depth) {
	const azimuth = (225 * Math.PI) / 180;
	const elevation = (60 * Math.PI) / 180;
	const lx = Math.cos(azimuth) * Math.cos(elevation);
	const ly = Math.sin(azimuth) * Math.cos(elevation);
	const lz = Math.sin(elevation);
	const albedo = 0.5 / lz;
	// Slopes are measured per device pixel; the relief is defined per CSS pixel.
	const scale = depth * density;
	const at = (x, y) => height[((y + size) % size) * size + ((x + size) % size)];

	const shade = new Float64Array(size * size);
	for (let y = 0; y < size; y++) {
		for (let x = 0; x < size; x++) {
			const dx =
				(at(x + 1, y - 1)
					+ 2 * at(x + 1, y)
					+ at(x + 1, y + 1)
					- at(x - 1, y - 1)
					- 2 * at(x - 1, y)
					- at(x - 1, y + 1))
				/ 4;
			const dy =
				(at(x - 1, y + 1)
					+ 2 * at(x, y + 1)
					+ at(x + 1, y + 1)
					- at(x - 1, y - 1)
					- 2 * at(x, y - 1)
					- at(x + 1, y - 1))
				/ 4;
			const nx = -scale * dx;
			const ny = -scale * dy;
			const light = (albedo * (nx * lx + ny * ly + lz)) / Math.hypot(nx, ny, 1);
			shade[y * size + x] = Math.min(1, Math.max(0, light));
		}
	}
	return shade;
}

// Encodes a square 8-bit PNG: one channel is gray, two are gray and alpha.
function png(size, channels, pixels) {
	const stride = size * channels;
	const rows = Buffer.alloc((stride + 1) * size);
	for (let y = 0; y < size; y++) {
		pixels.copy(rows, y * (stride + 1) + 1, y * stride, (y + 1) * stride);
	}

	const header = Buffer.alloc(13);
	header.writeUInt32BE(size, 0);
	header.writeUInt32BE(size, 4);
	header[8] = 8;
	header[9] = channels === 2 ? 4 : 0;

	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk('IHDR', header),
		chunk('IDAT', deflateSync(rows, { level: 9 })),
		chunk('IEND', Buffer.alloc(0)),
	]);
}

function chunk(type, data) {
	const length = Buffer.alloc(4);
	length.writeUInt32BE(data.length);
	const body = Buffer.concat([Buffer.from(type, 'ascii'), data]);
	const checksum = Buffer.alloc(4);
	checksum.writeUInt32BE(crc32(body));
	return Buffer.concat([length, body, checksum]);
}
