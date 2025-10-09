import { circle, path, polyline, rect, svg, symbol } from './lib/svg.js';

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
		path({ "d": "M5,53 s0,-15 15,-15 h60 s15,0 15,15 v32 s0,15 -15,15 h-60 s-15,0 -15,-15 z M45,78 l2,-8 c-7,-12 13,-12 6,0 l2,8 z", "fill": "#000", "stroke": "#000", "stroke-linejoin": "round", "fill-rule": "evenodd" }),
		path({
			"d": "M27,40 v-10 a1,1 0,0,1 46,0 v10",
			"fill": "none",
			"stroke": "currentColor",
			"stroke-width": "12",
		}),
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
	]),
	symbol({ "id": "remove", "viewBox": "0 0 32 34" }, path({
		"d": "M10,5 v-3 q0,-1 1,-1 h10 q1,0 1,1 v3 m8,0 h-28 q-1,0 -1,1 v2 q0,1 1,1 h28 q1,0 1,-1 v-2 q0,-1 -1,-1 m-2,4 v22 q0,2 -2,2 h-20 q-2,0 -2,-2 v-22 m2,3 v18 q0,1 1,1 h3 q1,0 1,-1 v-18 q0,-1 -1,-1 h-3 q-1,0 -1,1 m7.5,0 v18 q0,1 1,1 h3 q1,0 1,-1 v-18 q0,-1 -1,-1 h-3 q-1,0 -1,1 m7.5,0 v18 q0,1 1,1 h3 q1,0 1,-1 v-18 q0,-1 -1,-1 h-3 q-1,0 -1,1",
		"stroke": "currentColor",
		"fill": "none"
	})),
	symbol({ "id": "edit", "viewBox": "0 0 70 70", "fill": "none", "stroke": "#000" }, [
		polyline({
			"points": "51,7 58,0 69,11 62,18 51,7 7,52 18,63 62,18",
			"stroke-width": "2"
		}),
		path({ "d": "M7,52 L1,68 L18,63 M53,12 L14,51 M57,16 L18,55" })
	]),
	symbol({ "id": "goto", "viewBox": "0 0 70 70" }, [
		path({
			"d": "M45,15 l20,20 h-5 M45,55 l20,-20 h-65",
			"stroke-width": "10",
			"stroke": "#000",
			"fill": "none",
			"stroke-linejoin": "round",
			"stroke-linecap": "round"
		}),
	])
]);