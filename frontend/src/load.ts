import type { DirectoryWithChildren } from './types.js';
import { debouncer } from './lib/utils.js';
import Load from './data.js';
import { handleState, setState } from "./state.js";

export type LoadHandler = (path: string, data: DirectoryWithChildren) => void;

let lastPath = "/";

const debounce = debouncer<void>(),
	handlers: LoadHandler[] = [];

export const registerLoader = (fn: LoadHandler) => handlers.push(fn),
	load = (path = lastPath) => {
		return debounce(() => (setState("path", path), Load(path)).then(data => {
			lastPath = path;

			for (const fn of handlers) {
				fn(path, data);
			}
		}))
	};

handleState("path", path => load(path || lastPath), "/");
