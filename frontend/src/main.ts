import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';

// Svelte 5 bootstrap — must use mount(), not `new App(...)` (that crashes at runtime).
const app = mount(App, { target: document.getElementById('app')! });

export default app;
