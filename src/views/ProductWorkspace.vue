<script setup>
const sections = [
  {
    label: '商品档案',
    description: '日常维护',
    to: '/admin/products'
  },
  {
    label: '采购入库',
    description: '库存补充',
    to: '/admin/products/inbound'
  },
  {
    label: '库存流水',
    description: '变动追溯',
    to: '/admin/products/inventory-logs'
  }
]
</script>

<template>
  <section class="product-workspace">
    <nav class="product-section-nav" aria-label="商品板块功能">
      <RouterLink
        v-for="section in sections"
        :key="section.to"
        :to="section.to"
        class="product-section-link"
        exact-active-class="product-section-link--active"
      >
        <span class="product-section-label">{{ section.label }}</span>
        <span class="product-section-description">{{ section.description }}</span>
      </RouterLink>
    </nav>

    <RouterView />
  </section>
</template>

<style scoped>
.product-workspace {
  width: 100%;
}

.product-section-nav {
  display: flex;
  gap: 0.375rem;
  margin-bottom: 1.25rem;
  padding: 0.375rem;
  overflow-x: auto;
  border: 1px solid rgb(226 232 240 / 0.8);
  border-radius: 1rem;
  background: rgb(255 255 255 / 0.8);
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.04);
  scrollbar-width: none;
}

.product-section-nav::-webkit-scrollbar {
  display: none;
}

.product-section-link {
  display: flex;
  min-width: max-content;
  flex: 1 0 auto;
  align-items: baseline;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  border-radius: 0.75rem;
  color: rgb(100 116 139);
  text-decoration: none;
  transition: color 160ms ease, background-color 160ms ease, box-shadow 160ms ease;
}

.product-section-link:hover {
  color: rgb(15 23 42);
  background: rgb(248 250 252);
}

.product-section-link--active {
  color: white;
  background: rgb(15 23 42);
  box-shadow: 0 4px 12px rgb(15 23 42 / 0.16);
}

.product-section-label {
  font-size: 0.875rem;
  font-weight: 700;
  white-space: nowrap;
}

.product-section-description {
  font-size: 0.6875rem;
  color: rgb(148 163 184);
  white-space: nowrap;
}

.product-section-link--active .product-section-description {
  color: rgb(203 213 225);
}

@media (max-width: 767px) {
  .product-section-nav {
    margin-inline: -0.25rem;
    margin-bottom: 1rem;
  }

  .product-section-link {
    padding-inline: 0.875rem;
  }

  .product-section-description {
    display: none;
  }
}
</style>
