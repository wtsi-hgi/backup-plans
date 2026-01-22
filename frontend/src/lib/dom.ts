export type Children = string | Element | DocumentFragment | Children[];

export type Properties = Record<string, string | Function | boolean>;

export type PropertiesOrChildren = Properties | Children;

export const propertiesAndChildren = (propertiesOrChildren: PropertiesOrChildren, children?: Children): [Properties, Children] => typeof propertiesOrChildren === "string" || propertiesOrChildren instanceof Node || propertiesOrChildren instanceof Array ? [{}, propertiesOrChildren] : [propertiesOrChildren, children ?? []],
	amendNode = (node: Element, propertiesOrChildren: PropertiesOrChildren, children?: Children) => {
		const [p, c] = propertiesAndChildren(propertiesOrChildren, children);

		Object.entries(p).forEach(([key, value]) => node[value instanceof Function ? "addEventListener" : typeof value === "string" ? "setAttribute" : "toggleAttribute"](key, value as never));
		node.append(...[c as Element].flat(Infinity));

		return node;
	},
	clearNode = (node: Element, propertiesOrChildren: PropertiesOrChildren = {}, children?: Children) => amendNode((node.replaceChildren(), node), propertiesOrChildren, children),
	tags = <NS extends string>(ns: NS) => new Proxy({}, { "get": (_, element: string) => (props: PropertiesOrChildren = {}, children?: Children) => amendNode(document.createElementNS(ns, element), props, children) }) as NS extends "http://www.w3.org/1999/xhtml" ? { [K in keyof HTMLElementTagNameMap]: (props?: PropertiesOrChildren, children?: Children) => HTMLElementTagNameMap[K] } : NS extends "http://www.w3.org/2000/svg" ? { [K in keyof SVGElementTagNameMap]: (props?: PropertiesOrChildren, children?: Children) => SVGElementTagNameMap[K] } : Record<string, (props?: PropertiesOrChildren, children?: Children) => Element>;