import { circle, path, rect, svg, symbol } from './lib/svg.js';

export const symbols = svg({ "style": `width: 0; height: 0` }, [
	symbol({ "id": "ok", "viewBox": "0 0 100 100" }, [
		circle({
			"cx": "50",
			"cy": "50",
			"r": "45",
			"stroke": "currentColor",
			"fill": "none",
			"stroke-width": "10"
		}),
		path({
			"d": "M31,50 l13,13 l26,-26",
			"stroke": "currentColor",
			"fill": "none",
			"stroke-width": "10",
			"stroke-linecap": "round",
			"stroke-linejoin": "round"
		})
	]),
	symbol({ "id": "notok", "viewBox": "0 0 100 100" }, [
		circle({
			"cx": "50",
			"cy": "50",
			"r": "45",
			"stroke": "currentColor",
			"fill": "none",
			"stroke-width": "10"
		}),
		path({
			"d": "M35,35 l30,30 M35,65 l30,-30",
			"stroke": "currentColor",
			"fill": "none",
			"stroke-width": "10",
			"stroke-linecap": "round"
		})
	]),
	symbol({ "id": "lock", "viewBox": "0 0 100 100" }, [
		rect({
			"rx": "15",
			"x": "5",
			"y": "38",
			"width": "90",
			"height": "62",
			"fill": "currentColor"
		}),
		path({
			"d": "M27,40 v-10 a1,1 0,0,1 46,0 v10",
			"fill": "none",
			"stroke": "currentColor",
			"stroke-width": "12",
		})
	]),
	symbol({ "id": "emptyDirectory", "viewBox": "0 0 130 100" }, [
		path({
			"d": "M5,15 s0,-5 5,-5 h35 s5,0 5,5 s0,5 5,5 h35 s10,0 10,10 v10 h-65 s-6,0 -10,5 l-20,40 z M5,90 l20,-40 s4,-8 10,-8 h80 s12,0 10,10 l-20,40 s-3,10 -10,10 h-80 s-10,0 -10,-10 z",
			"fill": "currentColor"
		}),
		path({
			"d": "M103,10 l15,15 M118,10 l-15,15",
			"stroke": "currentColor",
			"fill": "none",
			"strokeWidth": "3",
			"stroke-linecap": "round"
		})
	])
]);