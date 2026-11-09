const themeButton = document.querySelector('.theme-toggle');
const setTheme = dark => {
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  themeButton.setAttribute('aria-pressed', String(dark));
  themeButton.setAttribute('aria-label', `Switch to ${dark ? 'light' : 'dark'} website theme`);
};
themeButton.addEventListener('click', () => setTheme(document.documentElement.dataset.theme !== 'dark'));

for (const link of document.querySelectorAll('[data-format]')) {
  link.addEventListener('click', event => {
    const input = document.getElementById(`format-${link.dataset.format}`);
    if (!input) return;
    event.preventDefault();
    input.checked = true;
    input.dispatchEvent(new Event('change', {bubbles: true}));
    input.focus({preventScroll: true});
    document.getElementById('artifact-demo').scrollIntoView({block: 'start'});
    history.replaceState(null, '', '#artifact-demo');
  });
}

const hero = document.querySelector('.hero');
const specimens = [...document.querySelectorAll('.specimen')];
const motion = matchMedia('(prefers-reduced-motion: reduce)');
const fine = matchMedia('(hover: hover) and (pointer: fine)');
let frame = 0;
let point;
hero.addEventListener('pointermove', event => {
  if (motion.matches || !fine.matches || event.pointerType === 'touch') return;
  point = {x: event.clientX, y: event.clientY};
  if (frame) return;
  frame = requestAnimationFrame(() => {
    frame = 0;
    const box = hero.getBoundingClientRect();
    const x = Math.max(-1, Math.min(1, (point.x - box.left) / box.width * 2 - 1));
    const y = Math.max(-1, Math.min(1, (point.y - box.top) / box.height * 2 - 1));
    specimens.forEach((card, index) => {
      const depth = [6, -8, 4][index];
      card.style.translate = `${x * depth}px ${y * depth}px`;
    });
  });
});
const resetDepth = () => {
  cancelAnimationFrame(frame);
  frame = 0;
  specimens.forEach(card => {card.style.removeProperty('translate');});
};
hero.addEventListener('pointerleave', resetDepth);
motion.addEventListener('change', resetDepth);
fine.addEventListener('change', resetDepth);

const agentSection = document.querySelector('.agents');
const agentMap = document.querySelector('.agent-map');
const agentSvg = document.querySelector('.agent-wires');
const agentClients = [...document.querySelectorAll('.agent-client')];
const agentHub = document.querySelector('.agent-hub');
const agentDestination = document.querySelector('.agent-destination');
let agentVisible = false;
let agentFrame = 0;
const syncAgentMotion = () => {
  agentSection.classList.toggle('is-paused', !agentVisible || document.hidden || motion.matches);
};
const drawAgentWires = () => {
  const map = agentMap.getBoundingClientRect();
  const hub = agentHub.getBoundingClientRect();
  const target = agentDestination.getBoundingClientRect();
  agentSvg.setAttribute('viewBox', `0 0 ${map.width} ${map.height}`);
  const fragment = document.createDocumentFragment();
  const pathPair = (d, index, trunk = false) => {
    for (const packet of [false, true]) {
      const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      path.setAttribute('d', d);
      path.setAttribute('class', `${packet ? 'agent-packet' : 'agent-wire'}${trunk ? ' agent-trunk' : ''}`);
      path.setAttribute('pathLength', '100');
      if (packet) path.style.animationDelay = `${index * -.7}s`;
      fragment.append(path);
    }
  };
  const hx = hub.left - map.left + hub.width / 2;
  const hy = hub.top - map.top;
  const compact = matchMedia('(max-width:900px)').matches;
  agentClients.forEach((client, index) => {
    const box = client.getBoundingClientRect();
    let x = box.left - map.left + box.width / 2;
    let y = box.bottom - map.top;
    let d;
    if (compact && index < 4) {
      const left = index % 2 === 0;
      x = (left ? box.left : box.right) - map.left;
      y = box.top - map.top + box.height / 2;
      const side = left ? 6 + index * 2 : map.width - 6 - index * 2;
      d = `M${x},${y} C${side},${y} ${side},${y} ${side},${y + 24} L${side},${hy - 26} Q${side},${hy + 12} ${hx},${hy + 12}`;
    } else {
      const endX = hx + (index - 2) * 9;
      const reach = Math.max(30, (hy - y) * .6);
      d = `M${x},${y} C${x},${y + reach} ${endX},${hy - reach} ${endX},${hy}`;
    }
    pathPair(d, index);
  });
  const fromY = hub.bottom - map.top;
  const toY = target.top - map.top;
  pathPair(`M${hx},${fromY} L${hx},${toY}`, 5, true);
  agentSvg.replaceChildren(fragment);
};
const scheduleAgentWires = () => {
  if (agentFrame) return;
  agentFrame = requestAnimationFrame(() => { agentFrame = 0; drawAgentWires(); });
};
new ResizeObserver(scheduleAgentWires).observe(agentMap);
document.fonts.ready.then(scheduleAgentWires);
new IntersectionObserver(entries => {
  agentVisible = entries[0].isIntersecting;
  syncAgentMotion();
}, {threshold: .1}).observe(agentSection);
motion.addEventListener('change', syncAgentMotion);
document.addEventListener('visibilitychange', syncAgentMotion);
syncAgentMotion();
