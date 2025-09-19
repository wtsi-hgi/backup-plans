const byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"];

export const formatBytes = (size: bigint) => {
	for (const suffix of byteSizes) {
		if (size < 1024n) {
			return size.toLocaleString() + suffix;
		}

		size >>= BigInt(10);
	}

	return "∞";
};