// A Vite plugin that renders the seamless saffiano leather tile behind the
// sign-in showcase. Stylesheets import it as `@generated/leather.png`; the
// plugin renders it into Vite's cache when Vite starts, and the build hashes
// and emits it like any other asset. The texture is a raster because an
// equivalent SVG filter makes browsers lag while painting the page.
//
// The tile is grayscale with flat areas at 50% gray, so a soft-light blend
// leaves the colors beneath unchanged and adds only the relief. Noise is
// shaped in the frequency domain, which makes it repeat seamlessly.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { crc32, deflateSync } from 'node:zlib';

// Tile size in device pixels; the stylesheet shows it at half size, so the
// grain stays sharp on high-density screens.
const size = 512;
const density = 2;
const seed = 7;

// Diagonal hatch cuts: frequencies in cycles per CSS pixel along and across
// the cut, and the direction of each cut set in degrees.
const hatch = { along: 0.07, across: 0.42, angles: [45, -45] };
// Fine pebble grain under the hatch, and its share of the relief.
const grain = { frequency: 0.22, weight: 0.6 };
// Relief depth: higher values make the leather more pronounced.
const depth = 1;
// Light direction, matching an SVG distant light.
const light = { azimuth: 225, elevation: 60 };
// Gray levels are rounded to this step. The soft-light blend hides steps this
// small, and fewer levels compress the PNG far better.
const step = 6;

const leatherImport = '@generated/leather.png';

const output = fileURLToPath(
	new URL('../node_modules/.vite/generated/leather.png', import.meta.url),
);

/** @returns {import('vite').Plugin} */
export function leatherTexture() {
	return {
		name: 'office:leather-texture',
		config() {
			mkdirSync(dirname(output), { recursive: true });
			writeFileSync(output, render());

			return { resolve: { alias: { [leatherImport]: output } } };
		},
	};
}

function random(state) {
	return () => {
		state = (state + 0x6d2b79f5) | 0;
		let t = Math.imul(state ^ (state >>> 15), 1 | state);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

// In-place radix-2 FFT over one row or column of a size×size grid.
function fft(re, im, offset, stride, inverse) {
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

function fft2(re, im, inverse) {
	for (let y = 0; y < size; y++) fft(re, im, y * size, 1, inverse);
	for (let x = 0; x < size; x++) fft(re, im, x, size, inverse);
}

// Seamless noise whose spectrum follows filter(fx, fy), normalized to zero
// mean and unit deviation. Frequencies are in cycles per device pixel.
function shapedNoise(next, filter) {
	const re = new Float64Array(size * size);
	const im = new Float64Array(size * size);
	for (let i = 0; i < re.length; i++) re[i] = next() - 0.5;

	fft2(re, im, false);
	for (let y = 0; y < size; y++) {
		const fy = (y < size / 2 ? y : y - size) / size;
		for (let x = 0; x < size; x++) {
			const fx = (x < size / 2 ? x : x - size) / size;
			const gain = filter(fx, fy);
			re[y * size + x] *= gain;
			im[y * size + x] *= gain;
		}
	}
	fft2(re, im, true);

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

// Turns noise into sparse raised ridges, like the SVG turbulence alpha ramp.
function ridges(noise, gain, cut) {
	return noise.map((z) =>
		Math.min(1, Math.max(0, (0.5 + z * 0.18) * gain - cut)),
	);
}

// Renders the tile as a grayscale PNG.
function render() {
	const next = random(seed);
	const layers = [];

	for (const degrees of hatch.angles) {
		const angle = (degrees * Math.PI) / 180;
		const along = hatch.along / density;
		const across = hatch.across / density;
		const noise = shapedNoise(next, (fx, fy) => {
			const u = fx * Math.cos(angle) + fy * Math.sin(angle);
			const v = -fx * Math.sin(angle) + fy * Math.cos(angle);
			return Math.exp(-((u / along) ** 2) - (v / across) ** 2);
		});
		layers.push({ height: ridges(noise, 2.2, 0.75), weight: 1 });
	}

	{
		const frequency = grain.frequency / density;
		const noise = shapedNoise(next, (fx, fy) =>
			Math.exp(-(fx * fx + fy * fy) / frequency ** 2),
		);
		layers.push({ height: ridges(noise, 1.4, 0.3), weight: grain.weight });
	}

	// Layers stack like translucent paint: each covers what is left uncovered.
	const height = new Float64Array(size * size);
	for (let i = 0; i < height.length; i++) {
		let uncovered = 1;
		for (const layer of layers) uncovered *= 1 - layer.height[i] * layer.weight;
		height[i] = 1 - uncovered;
	}

	// Diffuse lighting with a Sobel normal, wrapping at the edges. A flat surface
	// reflects exactly half of the light, which is the neutral soft-light gray.
	const azimuth = (light.azimuth * Math.PI) / 180;
	const elevation = (light.elevation * Math.PI) / 180;
	const lx = Math.cos(azimuth) * Math.cos(elevation);
	const ly = Math.sin(azimuth) * Math.cos(elevation);
	const lz = Math.sin(elevation);
	const albedo = 0.5 / lz;
	// Slopes are measured per device pixel; the relief is defined per CSS pixel.
	const scale = depth * density;
	const at = (x, y) => height[((y + size) % size) * size + ((x + size) % size)];

	const rows = Buffer.alloc(size * (size + 1));
	for (let y = 0; y < size; y++) {
		rows[y * (size + 1)] = 0;
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
			const shade = (albedo * (nx * lx + ny * ly + lz)) / Math.hypot(nx, ny, 1);
			const gray = Math.min(1, Math.max(0, shade)) * 255;
			rows[y * (size + 1) + 1 + x] = Math.min(
				255,
				Math.round(gray / step) * step,
			);
		}
	}

	const header = Buffer.alloc(13);
	header.writeUInt32BE(size, 0);
	header.writeUInt32BE(size, 4);
	header[8] = 8; // bit depth
	header[9] = 0; // grayscale

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
