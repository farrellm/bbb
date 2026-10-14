import { Route, Routes } from "react-router";
import RequireAuth from "./auth/RequireAuth";
import Layout from "./components/Layout";
import DiffPage from "./pages/DiffPage";
import Feed from "./pages/Feed";
import ItemDetail from "./pages/ItemDetail";
import Join from "./pages/Join";
import Login from "./pages/Login";
import NotFound from "./pages/NotFound";
import Settings from "./pages/Settings";
import SignedIn from "./pages/SignedIn";
import Timeline from "./pages/Timeline";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/join" element={<Join />} />
      <Route path="/signed-in" element={<SignedIn />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Feed />} />
        <Route path="/timeline" element={<Timeline />} />
        <Route path="/items/:panelID" element={<ItemDetail />} />
        <Route path="/items/:panelID/diff" element={<DiffPage />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}
