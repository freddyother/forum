document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('registerForm');
  if (form) {
    form.addEventListener('submit', (e) => {
      const p1 = document.getElementById('regPass');
      const p2 = document.getElementById('regPass2');
      const box = document.getElementById('regError');
      if (!p1 || !p2 || !box) return;
      if (p1.value.length < 6) {
        e.preventDefault();
        box.style.display = 'block';
        box.textContent = 'Password must be at least 6 characters.';
        p1.focus();
        return;
      }
      if (p1.value !== p2.value) {
        e.preventDefault();
        box.style.display = 'block';
        box.textContent = 'Passwords do not match.';
        p2.focus();
      }
    });
  }
});
