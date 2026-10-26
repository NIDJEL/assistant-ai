import { render, screen } from '@testing-library/react';
import { App } from '../../app/App';
import { mockServer } from '../../test/server';

it.each(['google', 'zoom'])('keeps the supported calendar usable when opening a retired %s bookmark', async provider => {
  history.pushState({}, '', `/integrations?service=${provider}`);
  mockServer(); render(<App/>);
  await screen.findByRole('heading', { name: 'Интеграции' });
  expect(screen.getByRole('heading', { name: 'Яндекс Календарь' })).toBeVisible();
  expect(screen.queryByRole('heading', { name: /Google|Zoom/ })).not.toBeInTheDocument();
  expect(screen.queryByRole('dialog', { name: 'Интеграция' })).not.toBeInTheDocument();
  expect(document.querySelector('a[href*="/api/auth/google"], a[href*="/api/auth/zoom"]')).toBeNull();
});
