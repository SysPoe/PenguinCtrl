export function installTimecodeNumberSteppers(root = document) {
  root.querySelectorAll('.timecode-grid input[type="number"]').forEach(input => {
    if (input.closest('.num-spin')) return;

    const wrap = document.createElement('span');
    wrap.className = 'num-spin';
    input.parentNode.insertBefore(wrap, input);
    wrap.appendChild(input);

    const controls = document.createElement('span');
    controls.className = 'spin-controls';
    controls.appendChild(stepButton(input, 'spin-up', 'Increase value', 1));
    controls.appendChild(stepButton(input, 'spin-down', 'Decrease value', -1));
    wrap.appendChild(controls);
  });
}

function stepButton(input, className, label, direction) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = className;
  button.setAttribute('aria-label', label);

  button.addEventListener('click', event => {
    event.preventDefault();
    event.stopPropagation();
    if (input.disabled) return;

    direction > 0 ? input.stepUp() : input.stepDown();
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });

  return button;
}
