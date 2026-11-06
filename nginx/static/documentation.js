(async () => {
  const container = document.getElementById('documentation');
  try {
    const response = await fetch('/documentation/raw');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const markdown = await response.text();
    container.innerHTML = DOMPurify.sanitize(marked.parse(markdown, { gfm: true }));
  } catch (error) {
    container.textContent = 'Не удалось загрузить документацию. Откройте исходный Markdown по ссылке выше.';
    console.error('Documentation loading failed:', error);
  }
})();
