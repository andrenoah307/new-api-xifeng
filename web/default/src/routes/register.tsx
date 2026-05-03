import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/register')({
  beforeLoad: ({ location }) => {
    throw redirect({
      to: '/sign-up',
      search: location.search,
    })
  },
})
