const byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"],
	secondsInDay = 86400,
	secondsInWeek = secondsInDay * 7;

export const formatBytes = (size: bigint) => {
	for (const suffix of byteSizes) {
		if (size < 1024n) {
			return size.toLocaleString() + suffix;
		}

		size >>= BigInt(10);
	}

	return "∞";
},
	longAgo = (unix: number) => {
		const diff = Math.max(((+new Date()) / 1000) - unix, 0),
			weeks = (diff / secondsInWeek) | 0;

		if (diff < secondsInWeek) {
			const days = (diff / secondsInDay) | 0;

			if (days === 1) {
				return "1 day";
			}

			return days + " days";
		}

		if (weeks == 1) {
			return "1 week";
		}

		return weeks + " weeks";
	}