import { Component, type ReactNode } from 'react';
import { ErrorPage } from '../pages/Errors/ErrorPage';

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? <ErrorPage status={500}/> : this.props.children; }
}
