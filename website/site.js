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
