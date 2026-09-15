export const routesNoauth = [
  {
    path: '/',
    name: 'root',
    children: [],
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/built-in/login/index.vue'),
    meta: {
      title: 'Login',
      withoutTab: true,
    },
  },
  {
    path: '/not-found',
    name: 'not-found',
    component: () => import('@/views/built-in/not-found/index.vue'),
    meta: {
      title: 'NotFound',
      icon: 'icon-park-outline:ghost',
      withoutTab: true,
    },
  },
  {
    path: '/:pathMatch(.*)*',
    component: () => import('@/views/built-in/not-found/index.vue'),
    name: 'not-found',
    meta: {
      title: 'NotFound',
      icon: 'icon-park-outline:ghost',
      withoutTab: true,
    },
  },
]
