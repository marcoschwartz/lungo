// Lungo — Client Runtime
// A minimal React-like framework: vdom, hooks, diffing, hydration, client router
// No build step required — uses tagged template literals
(function () {
  "use strict";

  // ─── Virtual DOM ───────────────────────────────────────────────────

  const EMPTY_OBJ = {};
  const EMPTY_ARR = [];
  const TEXT_NODE = "#text";

  function createVNode(tag, props, children) {
    return { tag, props: props || EMPTY_OBJ, children: children || EMPTY_ARR, _dom: null };
  }

  function createTextVNode(text) {
    return { tag: TEXT_NODE, props: EMPTY_OBJ, children: EMPTY_ARR, _text: String(text), _dom: null };
  }

  // ─── Tagged Template Parser ────────────────────────────────────────
  // Supports: <div>, <${Comp}>, </div>, </${Comp}>, <//>, <br/>, <${Comp} />
  // Caches parsed templates by strings array reference.

  const CACHE = new Map();
  const SLOT_MARKER = "\x00";

  function h(strings) {
    const args = arguments;

    // Direct call: h("div", props, ...) or h(Component, props, ...) or h(null, null, ...) for fragments
    if (typeof strings === "string" || typeof strings === "function" || strings === null) {
      if (strings === null) strings = Fragment;
      const props = arguments[1] || null;
      const children = EMPTY_ARR.slice.call(arguments, 2).map(normalizeChild).flat();
      return createVNode(strings, props, children);
    }

    // Tagged template: h`<div>...</div>`
    let tpl = CACHE.get(strings);
    if (!tpl) {
      tpl = parseTemplate(strings);
      CACHE.set(strings, tpl);
    }
    return buildTree(tpl, args);
  }

  function normalizeChild(c) {
    if (c == null || c === false || c === true) return [];
    if (Array.isArray(c)) return c.map(normalizeChild).flat();
    if (typeof c === "object" && c.tag) return [c];
    return [createTextVNode(c)];
  }

  // ─── Template Parser ──────────────────────────────────────────────
  // Builds the template string with slot markers, then tokenizes it.

  function parseTemplate(strings) {
    // Join strings with slot markers: "Hello \x001\x00 world"
    let html = strings[0];
    for (let i = 1; i < strings.length; i++) {
      html += SLOT_MARKER + (i - 1) + SLOT_MARKER + strings[i];
    }
    return tokenize(html);
  }

  // Tokenizer states
  const TEXT = 0, TAG_OPEN = 1, TAG_NAME = 2, ATTRS = 3, ATTR_NAME = 4,
        ATTR_EQ = 5, ATTR_VAL = 6, ATTR_VAL_DQ = 7, ATTR_VAL_SQ = 8,
        CLOSE_TAG = 9, CLOSE_TAG_NAME = 10;

  function tokenize(html) {
    const tokens = [];
    let i = 0;
    let textStart = 0;
    const len = html.length;

    while (i < len) {
      if (html[i] === "<") {
        // Flush text before this tag
        if (i > textStart) {
          const text = html.slice(textStart, i);
          if (text.trim()) tokens.push({ t: "text", v: text.trim() });
          else if (hasSlot(text)) tokens.push({ t: "text", v: text });
        }

        i++; // skip <
        if (i >= len) break;

        // Close tag?
        if (html[i] === "/") {
          i++; // skip /
          // Self-close shorthand: <//>
          if (i < len && html[i] === "/") {
            i++; // skip second /
            if (i < len && html[i] === ">") i++; // skip >
            tokens.push({ t: "close" });
            textStart = i;
            continue;
          }
          // Empty close: </>
          if (i < len && html[i] === ">") {
            i++; // skip >
            tokens.push({ t: "close" });
            textStart = i;
            continue;
          }
          // Named close tag: </div> or </${Comp}>
          let closeTag = "";
          while (i < len && html[i] !== ">") {
            closeTag += html[i];
            i++;
          }
          if (i < len) i++; // skip >
          tokens.push({ t: "close" });
          textStart = i;
          continue;
        }

        // Open tag — parse tag name and attributes
        let tagName = "";
        let slotIdx = -1;

        // Check for slot marker as tag name: <${Component}
        if (html[i] === SLOT_MARKER) {
          const slotEnd = html.indexOf(SLOT_MARKER, i + 1);
          slotIdx = parseInt(html.slice(i + 1, slotEnd));
          i = slotEnd + 1;
        } else {
          // Regular tag name
          while (i < len && html[i] !== " " && html[i] !== ">" && html[i] !== "/" && html[i] !== "\n" && html[i] !== "\t" && html[i] !== "\r") {
            tagName += html[i];
            i++;
          }
        }

        // Parse attributes
        const attrs = [];
        let selfClose = false;

        while (i < len) {
          // Skip whitespace
          while (i < len && (html[i] === " " || html[i] === "\n" || html[i] === "\t" || html[i] === "\r")) i++;

          if (i >= len) break;

          // Self-closing />
          if (html[i] === "/" && i + 1 < len && html[i + 1] === ">") {
            selfClose = true;
            i += 2;
            break;
          }

          // End of opening tag
          if (html[i] === ">") {
            i++;
            break;
          }

          // Attribute name
          let attrName = "";
          let attrSlotIdx = -1;

          if (html[i] === SLOT_MARKER) {
            // Spread or dynamic attr name (rare, skip for now)
            const se = html.indexOf(SLOT_MARKER, i + 1);
            attrSlotIdx = parseInt(html.slice(i + 1, se));
            i = se + 1;
            attrs.push({ spread: true, slotIndex: attrSlotIdx });
            continue;
          }

          while (i < len && html[i] !== "=" && html[i] !== " " && html[i] !== ">" && html[i] !== "/" && html[i] !== "\n" && html[i] !== "\t" && html[i] !== "\r") {
            attrName += html[i];
            i++;
          }

          if (!attrName) { i++; continue; }

          // Skip whitespace
          while (i < len && (html[i] === " " || html[i] === "\n" || html[i] === "\t" || html[i] === "\r")) i++;

          // No value — boolean attribute
          if (i >= len || html[i] !== "=") {
            attrs.push({ name: attrName, value: true });
            continue;
          }

          i++; // skip =

          // Skip whitespace after =
          while (i < len && (html[i] === " " || html[i] === "\n" || html[i] === "\t" || html[i] === "\r")) i++;

          // Attribute value
          if (i >= len) {
            attrs.push({ name: attrName, value: "" });
            break;
          }

          if (html[i] === SLOT_MARKER) {
            // Dynamic value: attr=${expr}
            const se = html.indexOf(SLOT_MARKER, i + 1);
            const si = parseInt(html.slice(i + 1, se));
            i = se + 1;
            attrs.push({ name: attrName, slotIndex: si });
          } else if (html[i] === '"') {
            i++;
            let val = "";
            while (i < len && html[i] !== '"') { val += html[i]; i++; }
            if (i < len) i++; // skip closing "
            attrs.push({ name: attrName, value: val });
          } else if (html[i] === "'") {
            i++;
            let val = "";
            while (i < len && html[i] !== "'") { val += html[i]; i++; }
            if (i < len) i++; // skip closing '
            attrs.push({ name: attrName, value: val });
          } else {
            // Unquoted value
            let val = "";
            while (i < len && html[i] !== " " && html[i] !== ">" && html[i] !== "\n" && html[i] !== "\t") {
              val += html[i]; i++;
            }
            attrs.push({ name: attrName, value: val });
          }
        }

        const tok = { t: "open", attrs, selfClose };
        if (slotIdx >= 0) {
          tok.slotIndex = slotIdx;
        } else {
          tok.tag = tagName;
        }
        tokens.push(tok);

        textStart = i;
      } else {
        i++;
      }
    }

    // Remaining text
    if (textStart < len) {
      const text = html.slice(textStart);
      if (text.trim() || hasSlot(text)) {
        tokens.push({ t: "text", v: text.trim() });
      }
    }

    return tokens;
  }

  function hasSlot(text) {
    return text.indexOf(SLOT_MARKER) >= 0;
  }

  function resolveSlots(text, args) {
    // Split on slot markers and resolve
    const parts = text.split(new RegExp(SLOT_MARKER + "(\\d+)" + SLOT_MARKER));
    if (parts.length === 1) return [createTextVNode(parts[0])];

    const result = [];
    for (let i = 0; i < parts.length; i++) {
      if (i % 2 === 0) {
        if (parts[i]) result.push(createTextVNode(parts[i]));
      } else {
        const val = args[parseInt(parts[i]) + 1];
        result.push(...normalizeChild(val));
      }
    }
    return result;
  }

  function buildTree(tokens, args) {
    const stack = [];
    let root = { tag: "__root", props: EMPTY_OBJ, children: [] };
    let current = root;
    stack.push(current);

    for (const tok of tokens) {
      if (tok.t === "open") {
        const tag = tok.slotIndex !== undefined ? args[tok.slotIndex + 1] : tok.tag;
        const props = {};

        for (const a of tok.attrs) {
          if (a.spread) {
            const spreadObj = args[a.slotIndex + 1];
            if (spreadObj && typeof spreadObj === "object") {
              Object.assign(props, spreadObj);
            }
            continue;
          }
          props[a.name] = a.slotIndex !== undefined ? args[a.slotIndex + 1] : a.value;
        }

        const node = createVNode(tag, props, []);
        current.children.push(node);

        if (!tok.selfClose) {
          stack.push(node);
          current = node;
        }
      } else if (tok.t === "close") {
        if (stack.length > 1) {
          stack.pop();
          current = stack[stack.length - 1];
        }
      } else if (tok.t === "text") {
        const children = resolveSlots(tok.v, args);
        current.children.push(...children);
      }
    }

    if (root.children.length === 1) return root.children[0];
    if (root.children.length === 0) return createTextVNode("");
    return createVNode("div", null, root.children);
  }

  // ─── Hooks ─────────────────────────────────────────────────────────

  let currentInstance = null;
  let hookIndex = 0;

  function getHook() {
    const inst = currentInstance;
    const idx = hookIndex++;
    if (!inst.__hooks) inst.__hooks = [];
    return [inst, idx];
  }

  function useState(initial) {
    const [inst, idx] = getHook();
    if (inst.__hooks.length <= idx) {
      inst.__hooks[idx] = typeof initial === "function" ? initial() : initial;
    }
    const capturedInst = inst;
    const setState = (val) => {
      const prev = capturedInst.__hooks[idx];
      const next = typeof val === "function" ? val(prev) : val;
      if (next !== prev) {
        capturedInst.__hooks[idx] = next;
        scheduleUpdate(capturedInst);
      }
    };
    return [inst.__hooks[idx], setState];
  }

  function useEffect(fn, deps) {
    const [inst, idx] = getHook();
    const prev = inst.__hooks[idx];
    const changed = !prev || !deps || deps.some((d, i) => d !== prev.deps[i]);
    inst.__hooks[idx] = { deps, fn, cleanup: prev ? prev.cleanup : null, changed };
  }

  function useMemo(fn, deps) {
    const [inst, idx] = getHook();
    const prev = inst.__hooks[idx];
    if (prev && deps && deps.every((d, i) => d === prev.deps[i])) return prev.value;
    const value = fn();
    inst.__hooks[idx] = { deps, value };
    return value;
  }

  function useRef(initial) {
    const [inst, idx] = getHook();
    if (inst.__hooks.length <= idx) {
      inst.__hooks[idx] = { current: initial };
    }
    return inst.__hooks[idx];
  }

  // ─── Component Instances ──────────────────────────────────────────

  const instances = new Map();

  function createInstance(component, props) {
    return {
      component,
      props,
      __hooks: [],
      __vnode: null,
      __dom: null,
      __parentDom: null,
    };
  }

  // ─── Render / Diff / Patch ────────────────────────────────────────

  let pendingUpdates = new Set();
  let updateScheduled = false;

  function scheduleUpdate(inst) {
    pendingUpdates.add(inst);
    if (!updateScheduled) {
      updateScheduled = true;
      queueMicrotask(flushUpdates);
    }
  }

  function flushUpdates() {
    updateScheduled = false;
    const updates = Array.from(pendingUpdates);
    pendingUpdates.clear();
    for (const inst of updates) {
      // Find the DOM node — may be detached after hydration patches
      let parentDom = inst.__parentDom;
      let dom = inst.__dom;
      if (!dom || !dom.parentNode) {
        // Try to find the instance's DOM via parent
        if (parentDom && parentDom.firstChild) {
          dom = parentDom.firstChild;
          inst.__dom = dom;
        }
      }
      if (dom) {
        const oldVNode = inst.__vnode;
        currentInstance = inst;
        hookIndex = 0;
        const newVNode = inst.component(inst.props);
        runEffects(inst);
        inst.__vnode = newVNode;
        currentInstance = null;
        patch(parentDom || dom.parentNode, oldVNode, newVNode, dom);
        // Update dom reference if changed
        const newDom = getDom(newVNode);
        if (newDom && newDom !== inst.__dom) {
          inst.__dom = newDom;
        }
      }
    }
  }

  function runEffects(inst) {
    if (!inst.__hooks) return;
    queueMicrotask(() => {
      for (const hook of inst.__hooks) {
        if (hook && hook.fn && hook.changed) {
          if (hook.cleanup) hook.cleanup();
          hook.cleanup = hook.fn() || null;
          hook.changed = false;
        }
      }
    });
  }

  function cleanupEffects(inst) {
    if (!inst.__hooks) return;
    for (const hook of inst.__hooks) {
      if (hook && hook.cleanup) hook.cleanup();
    }
  }

  function render(vnode, container) {
    const oldVNode = container.__vnode || null;
    if (oldVNode) {
      patch(container, oldVNode, vnode);
    } else {
      // First render: build new DOM then swap atomically (no flash)
      const dom = createDom(vnode, container);
      container.replaceChildren(dom);
    }
    container.__vnode = vnode;
  }

  function createDom(vnode, parentDom) {
    if (!vnode || typeof vnode !== "object") {
      return document.createTextNode(vnode == null ? "" : String(vnode));
    }

    if (vnode.tag === TEXT_NODE) {
      const dom = document.createTextNode(vnode._text);
      vnode._dom = dom;
      return dom;
    }

    // Component
    if (typeof vnode.tag === "function") {
      const children = vnode.children || [];
      const props = children.length > 0
        ? Object.assign({}, vnode.props, { children: children })
        : (vnode.props || {});
      const inst = createInstance(vnode.tag, props);
      currentInstance = inst;
      hookIndex = 0;
      const rendered = vnode.tag(props);
      runEffects(inst);
      inst.__vnode = rendered;
      currentInstance = null;

      const dom = createDom(rendered, parentDom);
      inst.__dom = dom;
      inst.__parentDom = parentDom;
      vnode._dom = dom;
      vnode._instance = inst;
      instances.set(dom, inst);
      return dom;
    }

    // Element — use SVG namespace for svg and its children
    const SVG_NS = "http://www.w3.org/2000/svg";
    const SVG_TAGS = new Set(["svg","path","circle","rect","line","polyline","polygon","ellipse","g","defs","use","text","tspan","clipPath","mask","filter","linearGradient","radialGradient","stop","feBlend","feColorMatrix","feFlood","feGaussianBlur","feMerge","feMergeNode","feOffset","animate","animateTransform","foreignObject","image","marker","pattern","symbol","textPath"]);
    const isSVG = SVG_TAGS.has(vnode.tag) || (parentDom && parentDom instanceof SVGElement);
    const dom = isSVG
      ? document.createElementNS(SVG_NS, vnode.tag)
      : document.createElement(vnode.tag);
    vnode._dom = dom;
    const props = vnode.props || EMPTY_OBJ;
    setProps(dom, EMPTY_OBJ, props, isSVG);

    // Handle ref
    if (props.ref && typeof props.ref === "object") {
      props.ref.current = dom;
    }

    if (vnode.children) {
      for (const child of vnode.children) {
        const childDom = createDom(child, dom);
        if (childDom) dom.appendChild(childDom);
      }
    }
    return dom;
  }

  function patch(parentDom, oldVNode, newVNode, existingDom) {
    if (oldVNode === newVNode) return;

    if (!oldVNode) {
      const dom = createDom(newVNode, parentDom);
      parentDom.appendChild(dom);
      return;
    }

    if (!newVNode) {
      removeDom(oldVNode);
      return;
    }

    // Different types — replace
    if (oldVNode.tag !== newVNode.tag) {
      const dom = createDom(newVNode, parentDom);
      const oldDom = existingDom || getDom(oldVNode);
      if (oldDom && oldDom.parentNode) {
        oldDom.parentNode.replaceChild(dom, oldDom);
      }
      cleanupInstance(oldVNode);
      return;
    }

    // Text node
    if (newVNode.tag === TEXT_NODE) {
      const dom = getDom(oldVNode);
      if (!dom) {
        // Missing DOM (aborted hydration) — create fresh
        const newDom = document.createTextNode(newVNode._text);
        newVNode._dom = newDom;
        if (parentDom) parentDom.appendChild(newDom);
        return;
      }
      if (oldVNode._text !== newVNode._text) {
        dom.textContent = newVNode._text;
      }
      newVNode._dom = dom;
      return;
    }

    // Component
    if (typeof newVNode.tag === "function") {
      const inst = oldVNode._instance;
      if (!inst) {
        // Missing instance (aborted hydration) — replace entire subtree
        const dom = createDom(newVNode, parentDom);
        const oldDom = existingDom || getDom(oldVNode);
        if (oldDom && oldDom.parentNode) {
          oldDom.parentNode.replaceChild(dom, oldDom);
        }
        return;
      }
      const props = newVNode.children.length > 0
        ? Object.assign({}, newVNode.props, { children: newVNode.children })
        : newVNode.props;
      inst.props = props;
      currentInstance = inst;
      hookIndex = 0;
      const rendered = newVNode.tag(props);
      runEffects(inst);
      const oldRendered = inst.__vnode;
      inst.__vnode = rendered;
      currentInstance = null;
      newVNode._instance = inst;
      newVNode._dom = inst.__dom;
      patch(inst.__parentDom, oldRendered, rendered, inst.__dom);
      const newDom = getDom(rendered);
      if (newDom && newDom !== inst.__dom) {
        inst.__dom = newDom;
        newVNode._dom = newDom;
      }
      return;
    }

    // Same element — diff props and children
    const dom = existingDom || getDom(oldVNode);
    if (!dom) {
      // Missing DOM — replace entirely
      const newDom = createDom(newVNode, parentDom);
      if (parentDom) parentDom.appendChild(newDom);
      return;
    }
    newVNode._dom = dom;
    setProps(dom, oldVNode.props, newVNode.props);
    // Skip child diffing for dangerouslySetInnerHTML — innerHTML was already set by setProps
    if (newVNode.props.dangerouslySetInnerHTML != null) return;
    diffChildren(dom, oldVNode.children, newVNode.children);
  }

  function diffChildren(parentDom, oldChildren, newChildren) {
    const max = Math.max(oldChildren.length, newChildren.length);
    for (let i = 0; i < max; i++) {
      const oldChild = oldChildren[i] || null;
      const newChild = newChildren[i] || null;
      const existingDom = oldChild ? getDom(oldChild) : null;

      if (!newChild && oldChild) {
        removeDom(oldChild);
      } else {
        patch(parentDom, oldChild, newChild, existingDom);
      }
    }
  }

  function getDom(vnode) {
    if (!vnode) return null;
    if (vnode._dom) return vnode._dom;
    if (vnode._instance) return vnode._instance.__dom;
    return null;
  }

  function removeDom(vnode) {
    const dom = getDom(vnode);
    if (dom && dom.parentNode) dom.parentNode.removeChild(dom);
    cleanupInstance(vnode);
  }

  function cleanupInstance(vnode) {
    if (vnode._instance) {
      cleanupEffects(vnode._instance);
      instances.delete(vnode._instance.__dom);
    }
    if (vnode.children) vnode.children.forEach(cleanupInstance);
  }

  function isSVGElement(dom) {
    return dom instanceof SVGElement;
  }

  function setProps(dom, oldProps, newProps, svg) {
    for (const key in oldProps) {
      if (key === "children" || key === "key" || key === "ref") continue;
      if (!(key in newProps)) {
        setProp(dom, key, null, svg);
      }
    }
    for (const key in newProps) {
      if (key === "children" || key === "key" || key === "ref") continue;
      if (newProps[key] !== oldProps[key]) {
        setProp(dom, key, newProps[key], svg);
      }
    }
  }

  function setProp(dom, key, value, svg) {
    // SVG elements: always use setAttribute (except event handlers and style)
    const isSVG = svg || isSVGElement(dom);

    if (key.startsWith("on")) {
      const event = key.slice(2).toLowerCase();
      dom["__rg_" + event] = value;
      if (!dom["__rg_has_" + event]) {
        dom["__rg_has_" + event] = true;
        dom.addEventListener(event, (e) => {
          const handler = dom["__rg_" + event];
          if (handler) handler(e);
        });
      }
    } else if (key === "style" && typeof value === "object") {
      Object.assign(dom.style, value);
    } else if (key === "className" || key === "class") {
      dom.setAttribute("class", value || "");
    } else if (key === "dangerouslySetInnerHTML") {
      dom.innerHTML = typeof value === "string" ? value : (value && value.__html) || "";
    } else if (!isSVG && (key === "value" || key === "checked" || key === "selected" || key === "disabled")) {
      dom[key] = value ?? "";
    } else if (isSVG) {
      // SVG attributes must use setAttribute
      if (value == null || value === false) {
        dom.removeAttribute(key);
      } else {
        dom.setAttribute(key, String(value));
      }
    } else if (key in dom && typeof value !== "string") {
      try { dom[key] = value ?? ""; } catch (e) { dom.setAttribute(key, value ?? ""); }
    } else if (value == null || value === false) {
      dom.removeAttribute(key);
    } else {
      dom.setAttribute(key, String(value));
    }
  }

  // ─── Hydration ────────────────────────────────────────────────────
  // Resilient hydration: reuses existing SSR DOM nodes where possible,
  // patches mismatches in-place instead of throwing and re-rendering.

  function hydrate(vnode, container) {
    hydrateNode(vnode, container.firstElementChild || container.firstChild, container);
    container.__vnode = vnode;
  }

  function hydrateNode(vnode, dom, parentDom) {
    if (!vnode || typeof vnode !== "object") return;
    if (!vnode.tag && vnode.tag !== null) return;

    // Text node
    if (vnode.tag === TEXT_NODE) {
      if (dom && dom.nodeType === 3) {
        // Reuse existing text node, update if different
        if (dom.textContent !== vnode._text) dom.textContent = vnode._text;
        vnode._dom = dom;
      } else if (dom && dom.nodeType !== 3) {
        // SSR has element where we expect text — find text sibling
        let textDom = dom;
        while (textDom && textDom.nodeType !== 3) textDom = textDom.nextSibling;
        if (textDom) {
          vnode._dom = textDom;
        } else {
          // No text node found — create one
          const newText = document.createTextNode(vnode._text);
          parentDom.insertBefore(newText, dom);
          vnode._dom = newText;
        }
      } else {
        // No DOM at all — create text node
        const newText = document.createTextNode(vnode._text || "");
        parentDom.appendChild(newText);
        vnode._dom = newText;
      }
      return;
    }

    // Skip whitespace-only text nodes to find matching element
    while (dom && dom.nodeType === 3 && !dom.textContent.trim()) {
      dom = dom.nextSibling;
    }

    // Component
    if (typeof vnode.tag === "function") {
      if (!dom) {
        // No SSR DOM — render from scratch into parent
        const newDom = createDom(vnode, parentDom);
        parentDom.appendChild(newDom);
        return;
      }
      const hChildren = vnode.children || [];
      const props = hChildren.length > 0
        ? Object.assign({}, vnode.props || {}, { children: hChildren })
        : (vnode.props || {});
      const inst = createInstance(vnode.tag, props);
      currentInstance = inst;
      hookIndex = 0;
      const rendered = vnode.tag(props);
      runEffects(inst);
      inst.__vnode = rendered;
      inst.__dom = dom;
      inst.__parentDom = parentDom;
      currentInstance = null;
      vnode._instance = inst;
      vnode._dom = dom;
      instances.set(dom, inst);
      hydrateNode(rendered, dom, parentDom);
      return;
    }

    // No DOM node — create element from scratch
    if (!dom) {
      const newDom = createDom(vnode, parentDom);
      parentDom.appendChild(newDom);
      return;
    }

    // Element — check tag match
    if (!vnode.tag || dom.nodeType !== 1 || dom.tagName.toLowerCase() !== vnode.tag.toLowerCase()) {
      // Mismatch — replace this node only (don't destroy entire tree)
      if (window.__LUNGO_DEV__) {
        console.warn("[Lungo] Hydration mismatch at <" + vnode.tag + ">: SSR has <" + (dom.tagName || "text").toLowerCase() + ">, patching in-place");
      }
      const newDom = createDom(vnode, parentDom);
      dom.parentNode.replaceChild(newDom, dom);
      return;
    }

    // Tag matches — reuse DOM node
    vnode._dom = dom;
    const hProps = vnode.props || EMPTY_OBJ;

    // dangerouslySetInnerHTML — skip child hydration, SSR HTML is already correct
    if (hProps.dangerouslySetInnerHTML != null) {
      setProps(dom, EMPTY_OBJ, hProps);
      return;
    }

    setProps(dom, EMPTY_OBJ, hProps);

    // Set refs
    if (hProps.ref && typeof hProps.ref === "object") {
      hProps.ref.current = dom;
    }

    // Hydrate children — resilient matching
    let childDom = dom.firstChild;
    const children = vnode.children;
    let i = 0;
    while (i < children.length) {
      const child = children[i];

      if (child.tag === TEXT_NODE) {
        // Collect consecutive text vnodes
        let textGroup = [child];
        while (i + 1 < children.length && children[i + 1].tag === TEXT_NODE) {
          i++;
          textGroup.push(children[i]);
        }

        const combinedText = textGroup.map(t => t._text).join("");
        if (!combinedText) {
          i++;
          continue;
        }

        // Find or create text node
        if (childDom && childDom.nodeType === 3) {
          for (const tv of textGroup) tv._dom = childDom;
          childDom = childDom.nextSibling;
        } else {
          // Create missing text node
          const newText = document.createTextNode(combinedText);
          if (childDom) {
            dom.insertBefore(newText, childDom);
          } else {
            dom.appendChild(newText);
          }
          for (const tv of textGroup) tv._dom = newText;
        }
      } else {
        // Element vnode — skip whitespace-only text DOM nodes
        while (childDom && childDom.nodeType === 3 && !childDom.textContent.trim()) {
          childDom = childDom.nextSibling;
        }

        if (!childDom) {
          // More vnode children than DOM — create remaining
          const newDom = createDom(child, dom);
          dom.appendChild(newDom);
        } else {
          hydrateNode(child, childDom, dom);
          childDom = childDom.nextSibling;
        }
      }
      i++;
    }
  }

  // ─── Client-Side Router ───────────────────────────────────────────

  let currentPath = location.pathname;

  // ─── Scroll Position Management ──────────────────────────────
  const scrollPositions = new Map();
  let isPopState = false;

  function saveScroll() {
    scrollPositions.set(currentPath, window.scrollY);
  }

  function restoreScroll(path) {
    if (isPopState && scrollPositions.has(path)) {
      // Back/forward — restore saved position
      requestAnimationFrame(() => window.scrollTo(0, scrollPositions.get(path)));
    } else {
      // Forward nav — scroll to top
      window.scrollTo(0, 0);
    }
    isPopState = false;
  }

  function useRouter() {
    const [path, setPath] = useState(currentPath);

    useEffect(() => {
      const handler = () => {
        isPopState = true;
        currentPath = location.pathname;
        setPath(currentPath);
      };
      window.addEventListener("popstate", handler);
      return () => window.removeEventListener("popstate", handler);
    }, []);

    return {
      pathname: path,
      query: Object.fromEntries(new URLSearchParams(location.search)),
      push(url) {
        saveScroll();
        history.pushState(null, "", url);
        currentPath = url;
        setPath(url);
      },
      replace(url) {
        history.replaceState(null, "", url);
        currentPath = url;
        setPath(url);
      },
      refresh() {
        // Increment global counter and dispatch event
        window.__LUNGO_REFRESH__ = (window.__LUNGO_REFRESH__ || 0) + 1;
        window.dispatchEvent(new CustomEvent("lungo:refresh"));
      },
    };
  }

  function navigate(url) {
    saveScroll();
    history.pushState(null, "", url);
    currentPath = url;
    window.dispatchEvent(new PopStateEvent("popstate"));
  }

  // ─── Link Prefetching ────────────────────────────────────────
  const prefetchedRoutes = new Set();

  function prefetchRoute(href) {
    if (prefetchedRoutes.has(href)) return;
    const routes = window.__LUNGO_ROUTES__ || [];
    for (const r of routes) {
      if (matchRoute(r.pattern, href)) {
        prefetchedRoutes.add(href);
        // Prefetch the page module
        const link = document.createElement("link");
        link.rel = "modulepreload";
        link.href = r.pagePath;
        document.head.appendChild(link);
        // Also prefetch loader data
        fetch("/_data" + href).catch(() => {});
        break;
      }
    }
  }

  // Prefetch links when they become visible
  if (typeof IntersectionObserver !== "undefined") {
    const prefetchObserver = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          const href = entry.target.getAttribute("href");
          if (href) prefetchRoute(href);
        }
      }
    }, { rootMargin: "200px" });

    // Observe links after page loads
    const observeLinks = () => {
      document.querySelectorAll("a[href]").forEach(link => {
        const href = link.getAttribute("href");
        if (href && !href.startsWith("http") && !href.startsWith("#") && isInternalRoute(href)) {
          prefetchObserver.observe(link);
        }
      });
    };

    // Re-observe after each render
    const origRender = render;
    render = function(vnode, container) {
      origRender(vnode, container);
      requestAnimationFrame(observeLinks);
    };
  }

  // Intercept <a> clicks for SPA navigation
  document.addEventListener("click", (e) => {
    const link = e.target.closest("a[href]");
    if (!link) return;
    const href = link.getAttribute("href");
    if (!href || href.startsWith("http") || href.startsWith("//") || href.startsWith("#")) return;
    if (link.target === "_blank" || e.metaKey || e.ctrlKey || e.shiftKey) return;
    if (!link.hasAttribute("data-link") && !isInternalRoute(href)) return;
    e.preventDefault();
    navigate(href);
  });

  // Also prefetch on hover for instant nav
  document.addEventListener("mouseover", (e) => {
    const link = e.target.closest("a[href]");
    if (!link) return;
    const href = link.getAttribute("href");
    if (href && !href.startsWith("http") && !href.startsWith("#")) {
      prefetchRoute(href);
    }
  });

  function isInternalRoute(href) {
    const routes = window.__LUNGO_ROUTES__ || [];
    const path = href.split("?")[0].split("#")[0];
    for (const r of routes) {
      if (matchRoute(r.pattern, path)) return true;
    }
    return false;
  }

  function matchRoute(pattern, path) {
    if (pattern === path) return true;
    const pp = pattern.split("/").filter(Boolean);
    const up = path.split("/").filter(Boolean);
    if (pp.length !== up.length) return false;
    return pp.every((p, i) => p.startsWith(":") || p === up[i]);
  }

  // ─── Streaming Support ───────────────────────────────────────────

  window.__RG_RESOLVE = function (id) {
    const tpl = document.querySelector(`template[data-chunk="${id}"]`);
    const placeholder = document.getElementById("rg-placeholder-" + id);
    if (tpl && placeholder) {
      const content = tpl.content.cloneNode(true);
      placeholder.replaceWith(content);
    }
  };

  // ─── HMR Client ──────────────────────────────────────────────────

  if (window.__LUNGO_DEV__) {
    try {
      const ws = new WebSocket(`ws://${location.host}/__hmr`);
      ws.onmessage = (e) => {
        const msg = JSON.parse(e.data);
        if (msg.type === "reload") {
          location.reload();
        }
      };
      ws.onclose = () => {
        console.log("[Lungo HMR] Disconnected, retrying in 2s...");
        setTimeout(() => location.reload(), 2000);
      };
    } catch (e) {
      // HMR not available
    }
  }

  // ─── Fragment ─────────────────────────────────────────────────────

  function Fragment(props) {
    return props.children;
  }

  // ─── Dynamic Page Router ──────────────────────────────────────────
  // Loads the correct page module based on the current URL path.
  // The SSR boot script sets up the initial page; on client-side nav,
  // this component dynamically imports the new page and re-renders.

  const pageModuleCache = new Map();

  function RouterView(props) {
    const router = useRouter();
    // Store page, data, params together so we can swap atomically
    const [view, setView] = useState(() => ({
      Page: window.Lungo?.__initialPage || null,
      data: window.Lungo?.__initialData || window.__LUNGO_DATA__ || {},
      params: window.__LUNGO_ROUTE__?.params || {},
      error: null,
    }));
    const layouts = props.layouts || [];
    const initialPath = useRef(window.__LUNGO_INITIAL_PATH__);
    const navId = useRef(0); // prevents stale async updates

    const [refreshKey, setRefreshKey] = useState(0);

    useEffect(() => {
      const onRefresh = () => setRefreshKey(k => k + 1);
      window.addEventListener("lungo:refresh", onRefresh);
      return () => window.removeEventListener("lungo:refresh", onRefresh);
    }, []);

    useEffect(() => {
      const isRefresh = refreshKey > 0;

      // Skip on initial render — SSR already loaded this page
      if (!isRefresh && router.pathname === initialPath.current) {
        initialPath.current = null;
        restoreScroll(router.pathname);
        return;
      }

      const routes = window.__LUNGO_ROUTES__ || [];
      let matched = null;
      let matchedParams = {};

      for (const r of routes) {
        const m = tryMatch(r.pattern, router.pathname);
        if (m) {
          matched = r;
          matchedParams = m;
          break;
        }
      }

      if (!matched) return;

      // Increment nav ID so stale loads are ignored
      const thisNav = ++navId.current;

      const loadPage = async () => {
        try {
          // Fetch server-rendered page fragment (like Next.js RSC)
          const pageRes = await fetch("/_page" + router.pathname);
          if (!pageRes.ok) throw new Error("Failed to load page");
          const pageData = await pageRes.json();

          // Ignore if a newer navigation happened while we were loading
          if (thisNav !== navId.current) return;

          // On refresh, also re-fetch layout loader data
          if (isRefresh) {
            try {
              const layoutRes = await fetch("/_data" + router.pathname + "?_layouts=1");
              if (layoutRes.ok) {
                const newLayoutData = await layoutRes.json();
                if (newLayoutData) {
                  for (const entry of layouts) {
                    if (entry && entry.component) {
                      for (const key of Object.keys(newLayoutData)) {
                        entry.data = newLayoutData[key];
                      }
                    }
                  }
                }
              }
            } catch (_) {}
          }

          // Also try to import the page module for client-side interactivity
          let PageComponent = null;
          try {
            const mod = pageModuleCache.get(matched.pagePath)
              || await import(matched.pagePath).then(m => { pageModuleCache.set(matched.pagePath, m); return m; });
            PageComponent = mod.default;
          } catch (_) {
            // Module import failed — use server-rendered HTML via a wrapper component
          }

          if (PageComponent) {
            // Interactive page — use the component directly
            setView({
              Page: PageComponent,
              data: pageData.data || {},
              params: matchedParams,
              error: null,
            });
          } else {
            // Server-only page — inject server-rendered HTML
            const ServerPage = () => h("div", { dangerouslySetInnerHTML: { __html: pageData.html } });
            setView({
              Page: ServerPage,
              data: pageData.data || {},
              params: matchedParams,
              error: null,
            });
          }

          // Restore or reset scroll after render
          if (!isRefresh) requestAnimationFrame(() => restoreScroll(router.pathname));
        } catch (err) {
          if (thisNav !== navId.current) return;
          setView(prev => ({ ...prev, error: err.message || "Failed to load page" }));
        }
      };

      loadPage();
    }, [router.pathname, refreshKey]);

    const { Page, data, params, error } = view;

    // Error state
    if (error) {
      let content = h`<div class="min-h-[50vh] flex items-center justify-center">
        <div class="text-center">
          <h2 class="text-2xl font-bold text-red-600 mb-2">Something went wrong</h2>
          <p class="text-gray-500">${error}</p>
        </div>
      </div>`;
      for (let i = layouts.length - 1; i >= 0; i--) {
        const entry = layouts[i];
        const Layout = entry && entry.component ? entry.component : entry;
        const lData = entry && entry.data ? entry.data : null;
        content = h`<${Layout} data=${lData}>${content}<//>`;
      }
      return content;
    }

    if (!Page) {
      return h`<div>Loading...</div>`;
    }

    // Normal render: Page wrapped in Layouts (layouts persist, only Page swaps)
    let content = h`<${Page} data=${data} params=${params} />`;
    for (let i = layouts.length - 1; i >= 0; i--) {
      const entry = layouts[i];
      const Layout = entry && entry.component ? entry.component : entry;
      const lData = entry && entry.data ? entry.data : null;
      content = h`<${Layout} data=${lData}>${content}<//>`;
    }
    return content;
  }

  function tryMatch(pattern, path) {
    if (pattern === path) return {};
    const pp = pattern.split("/").filter(Boolean);
    const up = path.split("/").filter(Boolean);
    if (pp.length !== up.length) return null;
    const params = {};
    for (let i = 0; i < pp.length; i++) {
      if (pp[i].startsWith(":")) {
        params[pp[i].slice(1)] = up[i];
      } else if (pp[i] !== up[i]) {
        return null;
      }
    }
    return params;
  }

  // ─── Exports ──────────────────────────────────────────────────────

  const Lungo = {
    h,
    Fragment,
    useState,
    useEffect,
    useMemo,
    useRef,
    render,
    hydrate,
    useRouter,
    navigate,
    createVNode,
    RouterView,
    LungoImage,
    Image: LungoImage,
  };

  // ─── Image Component ─────────────────────────────────────────────
  // Built-in component for optimized image loading.
  // Usage: <Image src="..." alt="..." priority />
  // Exported as LungoImage to avoid conflict with browser's Image constructor
  function LungoImage(props) {
    const imgProps = { src: props.src, alt: props.alt || "" };
    if (props.class) imgProps.class = props.class;
    if (props.width) imgProps.width = props.width;
    if (props.height) imgProps.height = props.height;
    if (props.style) imgProps.style = props.style;

    if (props.priority) {
      imgProps.loading = "eager";
      imgProps.fetchpriority = "high";
      imgProps.decoding = "sync";
    } else {
      imgProps.loading = "lazy";
      imgProps.decoding = "async";
    }

    // Blur placeholder
    if (props.placeholder === "blur" && props.blurDataURL) {
      imgProps.style = (imgProps.style || "") +
        ";background-image:url(" + props.blurDataURL + ");background-size:cover";
    }

    return h("img", imgProps);
  }

  window.Lungo = Lungo;
  if (typeof globalThis !== "undefined") globalThis.Lungo = Lungo;
})();
