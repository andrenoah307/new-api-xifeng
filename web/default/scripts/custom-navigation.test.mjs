import { test } from 'node:test'
import assert from 'node:assert/strict'
import * as defaultNavigation from '../src/lib/custom-navigation.ts'
import * as classicNavigation from '../../classic/src/helpers/custom-navigation.js'

for (const [theme, navigation] of [['default', defaultNavigation], ['classic', classicNavigation]]) {
  test(`${theme}: configured order, placement and opening mode survive parsing`, () => {
    const links = [
      { id: 'form', title: 'Business', url: 'https://example.com/form', mode: 'iframe', header: true, sidebar: false },
      { id: 'docs', title: 'Docs', url: 'http://example.com/docs', mode: 'external', header: false, sidebar: true },
    ]
    assert.deepEqual(navigation.parseCustomNavigationLinks(JSON.stringify(links)), links)
    assert.equal(navigation.customNavigationHref(links[0]), '/links/form')
    assert.equal(navigation.customNavigationHref(links[1]), 'http://example.com/docs')
  })
  test(`${theme}: invalid stored navigation never creates executable links`, () => {
    const link = { id: 'form', title: 'Form', url: 'https://example.com', mode: 'external', header: true, sidebar: true }
    for (const patch of [
      { url: 'javascript:alert(1)' }, { url: 'data:text/html,test' }, { url: '//example.com' },
      { url: 'https://user:password@example.com' }, { mode: 'iframe', url: 'http://example.com' },
      { id: '../admin' }, { mode: 'invalid' }, { title: '' },
    ]) assert.deepEqual(navigation.parseCustomNavigationLinks(JSON.stringify([{ ...link, ...patch }])), [])
    for (const raw of [undefined, '', 'null', '{}', 'invalid']) assert.deepEqual(navigation.parseCustomNavigationLinks(raw), [])
  })
}
