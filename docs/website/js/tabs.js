function switchTab(tabName) {
  // Hide all tab contents
  const contents = document.querySelectorAll('.tab-content');
  contents.forEach(content => content.classList.remove('active'));
  
  // Deactivate all tabs
  const tabs = document.querySelectorAll('.tab');
  tabs.forEach(tab => tab.classList.remove('active'));
  
  // Show selected tab content
  const selectedContent = document.getElementById(tabName);
  if (selectedContent) {
    selectedContent.classList.add('active');
  }
  
  // Activate selected tab
  const selectedTab = document.querySelector(`[onclick="switchTab('${tabName}')"]`);
  if (selectedTab) {
    selectedTab.classList.add('active');
  }
}

// Initialize first tab as active on load
document.addEventListener('DOMContentLoaded', () => {
  const firstTab = document.querySelector('.tab');
  if (firstTab) {
    firstTab.click();
  }
});
